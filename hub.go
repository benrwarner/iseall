package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const histLen = 300 // 5 minutes at 1s

type Alert struct {
	Level string `json:"level"` // "crit" | "warn"
	Key   string `json:"key"`
	Msg   string `json:"msg"`
	Nav   string `json:"nav,omitempty"`
}

type slowPayload struct {
	Mounts []Mount   `json:"mounts"`
	Alerts []Alert   `json:"alerts"`
	Docker *dSummary `json:"docker,omitempty"`
	Failed []string  `json:"failed"`
}

// hub samples the system and fans the result out to every connected browser over SSE.
// It samples every second while someone is watching and every 5s otherwise (to keep history warm).
type hub struct {
	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	wake   chan struct{}
	hist   []Sample
	info   SysInfo
	slow   []byte
	s      *sampler
	tick   int
	failed []string
	failAt time.Time
	trend  *trends
}

func newHub() *hub {
	return &hub{subs: map[chan []byte]struct{}{}, wake: make(chan struct{}, 1), s: newSampler(), info: readSysInfo(),
		trend: newTrends(filepath.Join(filepath.Dir(cfg.path), "history.json"))}
}

func sseFrame(event string, v any) []byte {
	b, _ := json.Marshal(v)
	var buf bytes.Buffer
	buf.Grow(len(b) + 32)
	buf.WriteString("event: ")
	buf.WriteString(event)
	buf.WriteString("\ndata: ")
	buf.Write(b)
	buf.WriteString("\n\n")
	return buf.Bytes()
}

func (h *hub) run() {
	h.s.sample() // prime counters
	for {
		h.mu.Lock()
		n := len(h.subs)
		h.mu.Unlock()
		d := 5 * time.Second
		if n > 0 {
			d = time.Second
		}
		select {
		case <-time.After(d):
		case <-h.wake:
			time.Sleep(200 * time.Millisecond)
		}
		smp := h.s.sample()
		h.trend.add(smp)
		h.mu.Lock()
		h.hist = append(h.hist, smp)
		if len(h.hist) > histLen {
			h.hist = h.hist[len(h.hist)-histLen:]
		}
		n = len(h.subs)
		h.mu.Unlock()
		if n == 0 {
			continue
		}
		h.broadcast(sseFrame("s", smp))
		if h.tick%5 == 0 {
			fr := sseFrame("slow", h.computeSlow(smp))
			h.mu.Lock()
			h.slow = fr
			h.mu.Unlock()
			h.broadcast(fr)
		}
		h.tick++
	}
}

func (h *hub) broadcast(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.subs {
		select {
		case c <- b:
		default: // slow client: drop a frame rather than block everyone
		}
	}
}

func (h *hub) computeSlow(smp Sample) slowPayload {
	cfg.mu.RLock()
	th := cfg.Alerts
	cfg.mu.RUnlock()
	sp := slowPayload{Mounts: readMounts(), Alerts: []Alert{}}
	if h.info.Docker {
		sp.Docker = dockerSummary()
	}
	if time.Since(h.failAt) > 15*time.Second {
		h.failed = failedUnits()
		h.failAt = time.Now()
	}
	sp.Failed = h.failed

	// sustained CPU: average of the last 10 samples
	h.mu.Lock()
	var cpuAvg float64
	k := 0
	for i := len(h.hist) - 1; i >= 0 && k < 10; i, k = i-1, k+1 {
		cpuAvg += h.hist[i].CPU
	}
	h.mu.Unlock()
	if k > 0 {
		cpuAvg /= float64(k)
	}
	add := func(level, key, nav, f string, a ...any) {
		sp.Alerts = append(sp.Alerts, Alert{Level: level, Key: key, Nav: nav, Msg: fmt.Sprintf(f, a...)})
	}
	if cpuAvg >= th.CPU {
		add("warn", "cpu", "processes", "CPU sustained at %.0f%%", cpuAvg)
	}
	if smp.MemT > 0 {
		if mp := float64(smp.MemU) * 100 / float64(smp.MemT); mp >= th.Mem {
			add("crit", "mem", "processes", "Memory at %.0f%%", mp)
		}
	}
	if smp.Temp >= th.Temp {
		lvl := "warn"
		if smp.Temp >= th.Temp+7 {
			lvl = "crit"
		}
		add(lvl, "temp", "overview", "CPU temperature %.1f°C", smp.Temp)
	}
	for _, m := range sp.Mounts {
		if m.Err != "" {
			add("warn", "mount:"+m.Path, "storage", "%s is not responding", m.Path)
		} else if m.Total > 0 {
			if p := float64(m.Used) * 100 / float64(m.Total); p >= th.Disk {
				add("crit", "disk:"+m.Path, "storage", "%s is %.0f%% full", m.Path, p)
			}
		}
	}
	if smp.Thr > 0 {
		if smp.Thr&0x1 != 0 {
			add("crit", "uv", "overview", "Under-voltage detected — check the power supply")
		} else if smp.Thr&0x10000 != 0 {
			add("warn", "uv-past", "overview", "Under-voltage occurred since boot")
		}
		if smp.Thr&0x4 != 0 {
			add("crit", "thr", "overview", "CPU is being throttled")
		} else if smp.Thr&0x8 != 0 {
			add("warn", "softtemp", "overview", "Soft temperature limit active")
		}
	}
	for _, u := range sp.Failed {
		add("crit", "unit:"+u, "services", "Service failed: %s", u)
	}
	if sp.Docker != nil {
		for _, c := range sp.Docker.Bad {
			add("warn", "ctr:"+c, "docker", "Container %s", c)
		}
	}
	sort.SliceStable(sp.Alerts, func(i, j int) bool { return sp.Alerts[i].Level == "crit" && sp.Alerts[j].Level != "crit" })
	return sp
}

func (h *hub) serveSSE(w http.ResponseWriter, r *http.Request) {
	fl, okF := w.(http.Flusher)
	if !okF {
		httpErr(w, 500, "streaming unsupported")
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Accel-Buffering", "no")

	c := make(chan []byte, 8)
	h.mu.Lock()
	// compact history rows: [t, cpu, mem%, temp, rx, tx, dr, dw]
	hist := make([][8]float64, len(h.hist))
	for i, s := range h.hist {
		mp := 0.0
		if s.MemT > 0 {
			mp = round1(float64(s.MemU) * 100 / float64(s.MemT))
		}
		hist[i] = [8]float64{float64(s.T), s.CPU, mp, s.Temp, s.Rx, s.Tx, s.DR, s.DW}
	}
	h.info.IPs = hostIPs()
	init := sseFrame("init", map[string]any{"info": h.info, "hist": hist})
	slow := h.slow
	h.subs[c] = struct{}{}
	h.mu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
	defer func() {
		h.mu.Lock()
		delete(h.subs, c)
		h.mu.Unlock()
	}()

	w.Write([]byte("retry: 2000\n\n"))
	w.Write(init)
	if slow != nil {
		w.Write(slow)
	}
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-c:
			if _, err := w.Write(b); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			w.Write([]byte(": ping\n\n"))
			fl.Flush()
		}
	}
}

// lineStream batches items from src and writes them as SSE "l" events (JSON arrays) every 120ms.
func lineStream(w http.ResponseWriter, r *http.Request, src <-chan any) {
	fl, _ := w.(http.Flusher)
	flush := func() {
		if fl != nil {
			fl.Flush()
		}
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Accel-Buffering", "no")
	w.Write([]byte("retry: 3000\n\n"))
	flush()
	var batch []any
	t := time.NewTicker(120 * time.Millisecond)
	defer t.Stop()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	send := func() bool {
		if len(batch) == 0 {
			return true
		}
		if _, err := w.Write(sseFrame("l", batch)); err != nil {
			return false
		}
		batch = batch[:0]
		flush()
		return true
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case l, open := <-src:
			if !open {
				send()
				w.Write([]byte("event: end\ndata: {}\n\n"))
				flush()
				return
			}
			batch = append(batch, l)
			if len(batch) >= 500 && !send() {
				return
			}
		case <-t.C:
			if !send() {
				return
			}
		case <-ping.C:
			w.Write([]byte(": ping\n\n"))
			flush()
		}
	}
}

func trimLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if len(s) > 4000 {
		s = s[:4000] + "…"
	}
	return s
}

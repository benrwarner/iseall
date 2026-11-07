package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// /api/pulse is a tiny, read-only health summary that other iseall instances on the tailnet fetch
// for their Tailscale page. It needs no login, so it is only answered for Tailscale source
// addresses, carries no process names, logs or controls, and can be switched off in Settings.

var tailnetNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func isTailnetAddr(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range tailnetNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

type pulse struct {
	Version  string     `json:"version"`
	Host     string     `json:"host"`
	Model    string     `json:"model"`
	Up       float64    `json:"up"`
	CPU      float64    `json:"cpu"`
	CPU24    float64    `json:"cpu24"`
	Mem      float64    `json:"mem"`
	MemU     uint64     `json:"memU"`
	MemT     uint64     `json:"memT"`
	Temp     float64    `json:"temp"`
	Load     [3]float64 `json:"load"`
	Cores    int        `json:"cores"`
	Disk     float64    `json:"disk"`
	DiskU    uint64     `json:"diskU"`
	DiskT    uint64     `json:"diskT"`
	Docker   bool       `json:"docker"`
	CtrRun   int        `json:"ctrRun"`
	CtrTotal int        `json:"ctrTotal"`
	Failed   int        `json:"failed"`
	Crit     int        `json:"crit"`
	Warn     int        `json:"warn"`
	Thr      int64      `json:"thr"`
	Spark    []float64  `json:"spark"`
}

func (h *hub) pulse() pulse {
	h.mu.Lock()
	var smp Sample
	if n := len(h.hist); n > 0 {
		smp = h.hist[n-1]
	}
	start := len(h.hist) - 60
	if start < 0 {
		start = 0
	}
	spark := make([]float64, 0, 60)
	for _, s := range h.hist[start:] {
		spark = append(spark, s.CPU)
	}
	info := h.info
	h.mu.Unlock()

	p := pulse{Version: version, Host: info.Hostname, Model: info.Model, Up: smp.Up, CPU: smp.CPU, MemU: smp.MemU, MemT: smp.MemT,
		Temp: smp.Temp, Load: smp.Load, Cores: info.Cores, Thr: smp.Thr, Spark: spark}
	if smp.MemT > 0 {
		p.Mem = round1(float64(smp.MemU) * 100 / float64(smp.MemT))
	}
	sp := h.freshSlow(smp, 20*time.Second)
	for _, m := range sp.Mounts {
		if m.Path == "/" && m.Total > 0 {
			p.DiskU, p.DiskT, p.Disk = m.Used, m.Total, round1(float64(m.Used)*100/float64(m.Total))
		}
	}
	if sp.Docker != nil {
		p.Docker, p.CtrRun, p.CtrTotal = true, sp.Docker.Running, sp.Docker.Total
	}
	p.Failed = len(sp.Failed)
	for _, a := range sp.Alerts {
		if a.Level == "crit" {
			p.Crit++
		} else {
			p.Warn++
		}
	}
	h.trend.mu.Lock()
	day := trendRanges["24h"].window
	p.CPU24 = summarize(h.trend.slice(true, time.Now().UnixMilli()-day), minStep, day).CPU.Avg
	h.trend.mu.Unlock()
	return p
}

func (h *hub) handlePulse(w http.ResponseWriter, r *http.Request) {
	if !isTailnetAddr(r.RemoteAddr) {
		httpErr(w, 403, "forbidden")
		return
	}
	cfg.mu.RLock()
	off := cfg.NoPulse
	cfg.mu.RUnlock()
	if off {
		httpErr(w, 403, "sharing disabled")
		return
	}
	writeJSON(w, r, h.pulse())
}

func handlePulseSetting(w http.ResponseWriter, r *http.Request) {
	var req struct{ Enabled bool }
	if readJSON(r, &req) != nil {
		httpErr(w, 400, "bad request")
		return
	}
	cfg.mu.Lock()
	cfg.NoPulse = !req.Enabled
	err := cfg.save()
	cfg.mu.Unlock()
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	ok(w, r)
}

// fetchPulse pulls a peer's health summary. The reason string explains a missing one to the UI:
// "update" (peer runs an iseall without /api/pulse), "disabled", "unreachable" or "error".
func fetchPulse(ip string) (json.RawMessage, string) {
	resp, err := tsClient.Get(peerURL(ip) + "api/pulse")
	if err != nil {
		return nil, "unreachable"
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	switch resp.StatusCode {
	case http.StatusOK:
		if json.Valid(b) {
			return b, ""
		}
		return nil, "error"
	case http.StatusNotFound:
		return nil, "update"
	case http.StatusForbidden:
		if strings.Contains(string(b), "disabled") {
			return nil, "disabled"
		}
	}
	return nil, "error"
}

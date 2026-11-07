package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Long-term history for rolling averages: 1-minute buckets for 24h and 15-minute buckets for 30d,
// each holding the average and peak of every sample that fell inside it. Persisted so reboots don't
// wipe it (~400 KB, written every 10 minutes and on shutdown).

const (
	minStep = int64(time.Minute / time.Millisecond)
	qStep   = int64(15 * time.Minute / time.Millisecond)
	minKeep = 24 * 60
	qKeep   = 30 * 24 * 4
)

// row layout on the wire and on disk
const (
	cT = iota
	cCPU
	cCPUMax
	cMem
	cMemMax
	cTemp
	cTempMax
	cLoad
	cRx
	cTx
	cDR
	cDW
	nCols
)

type row [nCols]float64

// open accumulates samples for the bucket that is still being filled.
type open struct {
	t   int64
	n   float64
	sum row
	max row
}

func (o *open) add(s Sample) {
	mp := 0.0
	if s.MemT > 0 {
		mp = float64(s.MemU) * 100 / float64(s.MemT)
	}
	v := row{0, s.CPU, s.CPU, mp, mp, s.Temp, s.Temp, s.Load[0], s.Rx, s.Tx, s.DR, s.DW}
	for i := 1; i < nCols; i++ {
		o.sum[i] += v[i]
		if v[i] > o.max[i] {
			o.max[i] = v[i]
		}
	}
	o.n++
}

func (o *open) row() row {
	var r row
	r[cT] = float64(o.t)
	for i := 1; i < nCols; i++ {
		r[i] = o.sum[i] / o.n
	}
	r[cCPUMax], r[cMemMax], r[cTempMax] = o.max[cCPUMax], o.max[cMemMax], o.max[cTempMax]
	for i := 1; i < nCols; i++ {
		if i >= cRx {
			r[i] = math.Round(r[i])
		} else {
			r[i] = round1(r[i])
		}
	}
	return r
}

type trends struct {
	mu     sync.Mutex
	min, q []row
	curM   open
	curQ   open
	path   string
	dirty  bool
}

func newTrends(path string) *trends {
	t := &trends{path: path}
	if b, err := os.ReadFile(path); err == nil {
		var d struct{ Min, Q []row }
		if json.Unmarshal(b, &d) == nil {
			now := time.Now().UnixMilli()
			t.min = keepSince(d.Min, now-minKeep*minStep)
			t.q = keepSince(d.Q, now-qKeep*qStep)
		}
	}
	go func() {
		for range time.Tick(10 * time.Minute) {
			t.save()
		}
	}()
	return t
}

func keepSince(rs []row, since int64) []row {
	for i, r := range rs {
		if int64(r[cT]) >= since {
			return rs[i:]
		}
	}
	return nil
}

func (t *trends) add(s Sample) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if m := s.T / minStep * minStep; m != t.curM.t {
		if t.curM.n > 0 {
			t.min = append(t.min, t.curM.row())
			if len(t.min) > minKeep {
				t.min = t.min[len(t.min)-minKeep:]
			}
			t.dirty = true
		}
		t.curM = open{t: m}
	}
	t.curM.add(s)
	if q := s.T / qStep * qStep; q != t.curQ.t {
		if t.curQ.n > 0 {
			t.q = append(t.q, t.curQ.row())
			if len(t.q) > qKeep {
				t.q = t.q[len(t.q)-qKeep:]
			}
		}
		t.curQ = open{t: q}
	}
	t.curQ.add(s)
}

func (t *trends) save() {
	t.mu.Lock()
	if !t.dirty {
		t.mu.Unlock()
		return
	}
	b, err := json.Marshal(map[string][]row{"min": t.min, "q": t.q})
	t.dirty = false
	t.mu.Unlock()
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(t.path), 0700)
	tmp := t.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		log.Printf("trends: save: %v", err)
		return
	}
	os.Rename(tmp, t.path)
}

// slice returns finished buckets since `since` plus the bucket still being filled.
func (t *trends) slice(fine bool, since int64) []row {
	src, cur := t.q, &t.curQ
	if fine {
		src, cur = t.min, &t.curM
	}
	rs := append([]row(nil), keepSince(src, since)...)
	if cur.n > 0 {
		rs = append(rs, cur.row())
	}
	return rs
}

type stat struct {
	Avg  float64 `json:"avg"`
	Peak float64 `json:"peak"`
}

type summary struct {
	CPU      stat    `json:"cpu"`
	Mem      stat    `json:"mem"`
	Temp     stat    `json:"temp"`
	Load     stat    `json:"load"`
	Rx       float64 `json:"rx"` // total bytes over the window
	Tx       float64 `json:"tx"`
	DR       float64 `json:"dr"`
	DW       float64 `json:"dw"`
	Coverage float64 `json:"coverage"` // share of the window that has data (0-1)
}

func summarize(rs []row, step, window int64) summary {
	var s summary
	if len(rs) == 0 {
		return s
	}
	sec := float64(step) / 1000
	for _, r := range rs {
		s.CPU.Avg += r[cCPU]
		s.Mem.Avg += r[cMem]
		s.Temp.Avg += r[cTemp]
		s.Load.Avg += r[cLoad]
		s.CPU.Peak = math.Max(s.CPU.Peak, r[cCPUMax])
		s.Mem.Peak = math.Max(s.Mem.Peak, r[cMemMax])
		s.Temp.Peak = math.Max(s.Temp.Peak, r[cTempMax])
		s.Load.Peak = math.Max(s.Load.Peak, r[cLoad])
		s.Rx += r[cRx] * sec
		s.Tx += r[cTx] * sec
		s.DR += r[cDR] * sec
		s.DW += r[cDW] * sec
	}
	n := float64(len(rs))
	s.CPU.Avg, s.Mem.Avg, s.Temp.Avg = round1(s.CPU.Avg/n), round1(s.Mem.Avg/n), round1(s.Temp.Avg/n)
	s.Load.Avg = math.Round(s.Load.Avg/n*100) / 100
	s.Coverage = math.Min(1, math.Round(n*float64(step)/float64(window)*100)/100)
	return s
}

var trendRanges = map[string]struct {
	window int64
	fine   bool
}{
	"1h":  {int64(time.Hour / time.Millisecond), true},
	"6h":  {int64(6 * time.Hour / time.Millisecond), true},
	"24h": {int64(24 * time.Hour / time.Millisecond), true},
	"7d":  {int64(7 * 24 * time.Hour / time.Millisecond), false},
	"30d": {int64(30 * 24 * time.Hour / time.Millisecond), false},
}

func (h *hub) handleTrends(w http.ResponseWriter, r *http.Request) {
	t := h.trend
	now := time.Now().UnixMilli()
	t.mu.Lock()
	defer t.mu.Unlock()

	// compact rolling averages for the overview card
	if r.URL.Query().Get("summary") == "1" {
		out := map[string]summary{}
		for _, k := range []string{"1h", "24h", "7d", "30d"} {
			rg := trendRanges[k]
			step := qStep
			if rg.fine {
				step = minStep
			}
			out[k] = summarize(t.slice(rg.fine, now-rg.window), step, rg.window)
		}
		writeJSON(w, r, out)
		return
	}

	key := r.URL.Query().Get("range")
	rg, okR := trendRanges[key]
	if !okR {
		key, rg = "24h", trendRanges["24h"]
	}
	step := qStep
	if rg.fine {
		step = minStep
	}
	rows := t.slice(rg.fine, now-rg.window)

	// average CPU by local hour of day over the last 7 days — "when is this box busy?"
	var hours [24]float64
	var cnt [24]float64
	for _, q := range t.slice(false, now-trendRanges["7d"].window) {
		hr := time.UnixMilli(int64(q[cT])).Hour()
		hours[hr] += q[cCPU]
		cnt[hr]++
	}
	for i := range hours {
		if cnt[i] > 0 {
			hours[i] = round1(hours[i] / cnt[i])
		} else {
			hours[i] = -1 // no data yet
		}
	}
	writeJSON(w, r, map[string]any{
		"range": key, "step": step, "window": rg.window, "rows": rows,
		"summary": summarize(rows, step, rg.window), "hours": hours, "cores": h.info.Cores,
	})
}

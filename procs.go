package main

import (
	"bytes"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type procInfo struct {
	PID     int     `json:"pid"`
	PPID    int     `json:"ppid"`
	Name    string  `json:"name"`
	User    string  `json:"user"`
	State   string  `json:"st"`
	CPU     float64 `json:"cpu"`
	RSS     uint64  `json:"rss"`
	Threads int     `json:"thr"`
	Nice    int     `json:"ni"`
	Start   int64   `json:"start"`
	Cmd     string  `json:"cmd"`
	Kernel  bool    `json:"k,omitempty"`
	Ctr     string  `json:"ctr,omitempty"`
	Unit    string  `json:"unit,omitempty"`
}

// Per-process cache keyed by pid; invalidated when starttime changes (pid reuse).
// cmdline and cgroup almost never change, so we read them once per process lifetime.
type procCache struct {
	start uint64
	ticks uint64
	cmd   string
	ctrID string
	unit  string
}

var procs = struct {
	sync.Mutex
	prev  map[int]*procCache
	prevT time.Time
}{prev: map[int]*procCache{}}

const clkTck = 100

var pageSize = uint64(os.Getpagesize())

// ---------- /etc/passwd ----------

var users = struct {
	sync.Mutex
	m     map[uint32]string
	mtime time.Time
}{}

func refreshUsers() {
	if st, err := os.Stat("/etc/passwd"); err == nil && st.ModTime() != users.mtime {
		users.mtime = st.ModTime()
		users.m = map[uint32]string{}
		b, _ := os.ReadFile("/etc/passwd")
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Split(l, ":")
			if len(f) > 3 {
				id, _ := strconv.ParseUint(f[2], 10, 32)
				users.m[uint32(id)] = f[0]
			}
		}
	}
}

func userName(uid uint32) string {
	users.Lock()
	defer users.Unlock()
	refreshUsers()
	if n, ok := users.m[uid]; ok {
		return n
	}
	return strconv.FormatUint(uint64(uid), 10)
}

func userExists(name string) bool {
	users.Lock()
	defer users.Unlock()
	refreshUsers()
	for _, n := range users.m {
		if n == name {
			return true
		}
	}
	return false
}

// ---------- cgroup → container / systemd unit ----------

var reDockerID = regexp.MustCompile(`(?:docker[-/]|/libpod-)([0-9a-f]{64})`)

func cgroupTags(pid int) (ctrID, unit string) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		return
	}
	if m := reDockerID.FindSubmatch(b); m != nil {
		return string(m[1]), ""
	}
	// last "*.service" segment of the unified (or name=systemd) hierarchy
	for _, l := range bytes.Split(b, []byte{'\n'}) {
		if !bytes.HasPrefix(l, []byte("0::")) && !bytes.Contains(l, []byte("name=systemd")) {
			continue
		}
		for _, seg := range strings.Split(string(l), "/") {
			if strings.HasSuffix(seg, ".service") && !strings.HasPrefix(seg, "user@") {
				unit = seg
			}
		}
	}
	return
}

// ---------- listing ----------

func parseStat(b []byte) (name string, f []string, ok bool) {
	i := bytes.IndexByte(b, '(')
	j := bytes.LastIndexByte(b, ')')
	if i < 0 || j < i || j+2 > len(b) {
		return "", nil, false
	}
	return string(b[i+1 : j]), strings.Fields(string(b[j+2:])), true
}

func listProcs(bootTime int64) []procInfo {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	names := ctrNames()
	procs.Lock()
	defer procs.Unlock()
	now := time.Now()
	dt := now.Sub(procs.prevT).Seconds()
	procs.prevT = now
	next := make(map[int]*procCache, len(procs.prev))
	out := make([]procInfo, 0, len(ents))
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := "/proc/" + e.Name()
		b, err := os.ReadFile(dir + "/stat")
		if err != nil {
			continue
		}
		name, f, ok := parseStat(b)
		if !ok || len(f) < 22 {
			continue
		}
		ppid, _ := strconv.Atoi(f[1])
		ut, _ := strconv.ParseUint(f[11], 10, 64)
		st, _ := strconv.ParseUint(f[12], 10, 64)
		nice, _ := strconv.Atoi(f[16])
		thr, _ := strconv.Atoi(f[17])
		start, _ := strconv.ParseUint(f[19], 10, 64)
		rss, _ := strconv.ParseUint(f[21], 10, 64)
		ticks := ut + st

		p := procInfo{PID: pid, PPID: ppid, Name: name, State: f[0], Nice: nice, Threads: thr, RSS: rss * pageSize,
			Start: bootTime + int64(start/clkTck), Kernel: pid == 2 || ppid == 2}

		c := procs.prev[pid]
		if c != nil && c.start == start {
			if dt > 0 && ticks >= c.ticks {
				p.CPU = round1(float64(ticks-c.ticks) / clkTck / dt * 100)
			}
		} else {
			c = &procCache{start: start}
			if !p.Kernel {
				cl, _ := os.ReadFile(dir + "/cmdline")
				cl = bytes.TrimRight(cl, "\x00")
				for i := range cl {
					if cl[i] == 0 {
						cl[i] = ' '
					}
				}
				if len(cl) > 600 {
					cl = append(cl[:600], "…"...)
				}
				c.cmd = string(cl)
				c.ctrID, c.unit = cgroupTags(pid)
			}
		}
		c.ticks = ticks
		next[pid] = c
		p.Cmd = c.cmd
		p.Unit = c.unit
		if c.ctrID != "" {
			p.Ctr = names[c.ctrID]
			if p.Ctr == "" {
				p.Ctr = c.ctrID[:12]
			}
		}
		if fi, err := e.Info(); err == nil {
			if s, ok := fi.Sys().(*syscall.Stat_t); ok {
				p.User = userName(s.Uid)
			}
		}
		out = append(out, p)
	}
	procs.prev = next
	return out
}

func (h *hub) handleProcs(w http.ResponseWriter, r *http.Request) {
	ps := listProcs(h.info.BootTime)
	mt := uint64(0)
	if mi := readMeminfo(); mi != nil {
		mt = mi["MemTotal"]
	}
	writeJSON(w, r, map[string]any{"procs": ps, "memTotal": mt, "cores": h.info.Cores})
}

func handleProcDetail(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil {
		httpErr(w, 400, "bad pid")
		return
	}
	dir := "/proc/" + strconv.Itoa(pid)
	d := map[string]any{"pid": pid}
	d["cwd"], _ = os.Readlink(dir + "/cwd")
	d["exe"], _ = os.Readlink(dir + "/exe")
	if fds, err := os.ReadDir(dir + "/fd"); err == nil {
		d["fds"] = len(fds)
	}
	if b, err := os.ReadFile(dir + "/io"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, _ := strings.Cut(l, ": ")
			if k == "read_bytes" || k == "write_bytes" {
				d[k], _ = strconv.ParseUint(v, 10, 64)
			}
		}
	}
	if b, err := os.ReadFile(dir + "/status"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, _ := strings.Cut(l, ":")
			switch k {
			case "VmPeak", "VmSwap", "voluntary_ctxt_switches", "nonvoluntary_ctxt_switches":
				d[k] = strings.TrimSpace(v)
			}
		}
	}
	cl, _ := os.ReadFile(dir + "/cmdline")
	d["argv"] = strings.Split(strings.TrimRight(string(cl), "\x00"), "\x00")
	var kids []int
	ps := strconv.Itoa(pid)
	ents, _ := os.ReadDir("/proc")
	for _, e := range ents {
		cp, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, _ := os.ReadFile("/proc/" + e.Name() + "/stat")
		if _, f, ok := parseStat(b); ok && len(f) > 1 && f[1] == ps {
			kids = append(kids, cp)
		}
	}
	d["children"] = kids
	writeJSON(w, r, d)
}

var signals = map[string]syscall.Signal{"TERM": syscall.SIGTERM, "KILL": syscall.SIGKILL, "HUP": syscall.SIGHUP,
	"INT": syscall.SIGINT, "STOP": syscall.SIGSTOP, "CONT": syscall.SIGCONT, "USR1": syscall.SIGUSR1, "USR2": syscall.SIGUSR2}

func handleSignal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PID  int    `json:"pid"`
		Sig  string `json:"sig"`
		Nice *int   `json:"nice"`
	}
	if readJSON(r, &req) != nil || req.PID <= 1 || req.PID == os.Getpid() {
		httpErr(w, 400, "refusing to signal that process")
		return
	}
	if req.Nice != nil {
		if err := syscall.Setpriority(syscall.PRIO_PROCESS, req.PID, *req.Nice); err != nil {
			httpErr(w, 500, err.Error())
			return
		}
		ok(w, r)
		return
	}
	sig, found := signals[req.Sig]
	if !found {
		httpErr(w, 400, "unknown signal")
		return
	}
	if err := syscall.Kill(req.PID, sig); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	ok(w, r)
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type cmdErr struct{ msg string }

func (e *cmdErr) Error() string { return e.msg }

func systemctl(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "systemctl", append([]string{"--no-pager"}, args...)...).CombinedOutput()
	if err != nil && len(out) > 0 {
		return string(out), &cmdErr{strings.TrimSpace(string(out))}
	}
	return string(out), err
}

func failedUnits() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, _ := systemctl(ctx, "list-units", "--state=failed", "--plain", "--no-legend")
	res := []string{}
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			res = append(res, f[0])
		}
	}
	return res
}

type service struct {
	Unit   string  `json:"unit"`
	Load   string  `json:"load"`
	Active string  `json:"active"`
	Sub    string  `json:"sub"`
	Desc   string  `json:"desc"`
	File   string  `json:"file"` // enabled | disabled | static | masked | ...
	Mem    uint64  `json:"mem"`
	CPU    float64 `json:"cpu"`
	Tasks  int     `json:"tasks"`
	Mine   bool    `json:"mine,omitempty"`
}

var svcCPU = struct {
	sync.Mutex
	prev map[string]cgStat
}{prev: map[string]cgStat{}}

func handleServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var units, files string
	var err error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		units, err = systemctl(ctx, "list-units", "--type=service", "--all", "--plain", "--no-legend")
	}()
	go func() {
		defer wg.Done()
		files, _ = systemctl(ctx, "list-unit-files", "--type=service", "--plain", "--no-legend")
	}()
	wg.Wait()
	if err != nil {
		httpErr(w, 500, "systemctl: "+err.Error())
		return
	}
	fileState := map[string]string{}
	for _, l := range strings.Split(files, "\n") {
		if f := strings.Fields(l); len(f) >= 2 {
			fileState[f[0]] = f[1]
		}
	}
	m := map[string]*service{}
	for _, l := range strings.Split(units, "\n") {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		m[f[0]] = &service{Unit: f[0], Load: f[1], Active: f[2], Sub: f[3], Desc: strings.Join(f[4:], " "), File: fileState[f[0]]}
	}
	// unit files that aren't loaded still matter (e.g. disabled services you may want to start)
	for u, st := range fileState {
		if _, ok := m[u]; !ok && !strings.Contains(u, "@.") {
			m[u] = &service{Unit: u, Load: "not-loaded", Active: "inactive", Sub: "dead", File: st}
		}
	}
	now := time.Now()
	svcCPU.Lock()
	out := make([]service, 0, len(m))
	for _, s := range m {
		s.Mine = strings.HasPrefix(s.Unit, "iseall-")
		if s.Active == "active" || s.Active == "activating" || s.Active == "reloading" {
			dir := "/sys/fs/cgroup/system.slice/" + s.Unit
			if b, err := os.ReadFile(dir + "/cpu.stat"); err == nil {
				var usage uint64
				for _, l := range strings.Split(string(b), "\n") {
					if v, ok := strings.CutPrefix(l, "usage_usec "); ok {
						usage, _ = strconv.ParseUint(v, 10, 64)
						usage *= 1000
					}
				}
				if p, ok := svcCPU.prev[s.Unit]; ok && usage >= p.usage {
					if el := now.Sub(p.at).Nanoseconds(); el > 0 {
						s.CPU = round1(float64(usage-p.usage) / float64(el) * 100)
					}
				}
				svcCPU.prev[s.Unit] = cgStat{usage: usage, at: now}
				s.Mem, _ = strconv.ParseUint(readTrim(dir+"/memory.current"), 10, 64)
				s.Tasks, _ = strconv.Atoi(readTrim(dir + "/pids.current"))
			}
		}
		out = append(out, *s)
	}
	svcCPU.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Unit < out[j].Unit })
	writeJSON(w, r, out)
}

var reUnit = regexp.MustCompile(`^[a-zA-Z0-9:_.@\\-]{1,200}\.(service|timer|socket|path|mount|target)$`)

func handleServiceAction(w http.ResponseWriter, r *http.Request) {
	unit, action := r.PathValue("unit"), r.PathValue("action")
	if !reUnit.MatchString(unit) {
		httpErr(w, 400, "bad unit name")
		return
	}
	var args []string
	switch action {
	case "start", "stop", "restart", "reload", "enable", "disable", "mask", "unmask", "reset-failed":
		args = []string{action, unit}
	case "enable-now":
		args = []string{"enable", "--now", unit}
	case "disable-now":
		args = []string{"disable", "--now", unit}
	default:
		httpErr(w, 400, "unknown action")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if _, err := systemctl(ctx, args...); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	ok(w, r)
}

func handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	unit := r.PathValue("unit")
	if !reUnit.MatchString(unit) {
		httpErr(w, 400, "bad unit name")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	props := "Id,Description,LoadState,ActiveState,SubState,UnitFileState,FragmentPath,MainPID,ExecMainStartTimestamp,ActiveEnterTimestamp," +
		"NRestarts,Restart,User,Group,WorkingDirectory,Result,After,WantedBy,Documentation,MemoryCurrent,TasksCurrent,CPUUsageNSec"
	show, err := systemctl(ctx, "show", unit, "-p", props)
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	p := map[string]string{}
	for _, l := range strings.Split(show, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			p[k] = v
		}
	}
	cat, _ := systemctl(ctx, "cat", unit)
	if len(cat) > 64<<10 {
		cat = cat[:64<<10]
	}
	writeJSON(w, r, map[string]any{"props": p, "file": cat})
}

// ---------- journal ----------

type jEntry struct {
	TS    string          `json:"__REALTIME_TIMESTAMP"`
	Prio  string          `json:"PRIORITY"`
	Ident string          `json:"SYSLOG_IDENTIFIER"`
	Comm  string          `json:"_COMM"`
	Unit  string          `json:"_SYSTEMD_UNIT"`
	Msg   json.RawMessage `json:"MESSAGE"`
}

func jMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '[' { // non-UTF8 payloads arrive as a byte array
		var ints []int
		json.Unmarshal(raw, &ints)
		bs := make([]byte, len(ints))
		for i, v := range ints {
			bs[i] = byte(v)
		}
		return string(bs)
	}
	var s string
	json.Unmarshal(raw, &s)
	return s
}

var reSince = regexp.MustCompile(`^-?\d+ ?(min|h|d|hours?|days?)( ago)?$|^today$|^yesterday$`)

func handleJournal(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("n"))
	if n <= 0 || n > 5000 {
		n = 300
	}
	args := []string{"-o", "json", "--output-fields=MESSAGE,PRIORITY,SYSLOG_IDENTIFIER,_COMM,_SYSTEMD_UNIT", "--no-pager", "-n", strconv.Itoa(n)}
	if q.Get("follow") != "0" {
		args = append(args, "-f")
	}
	if u := q.Get("unit"); u != "" {
		if !reUnit.MatchString(u) {
			httpErr(w, 400, "bad unit")
			return
		}
		args = append(args, "-u", u)
	}
	if p := q.Get("prio"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 0 && v <= 7 {
			args = append(args, "-p", p)
		}
	}
	if q.Get("boot") == "1" {
		args = append(args, "-b")
	}
	if g := q.Get("grep"); g != "" && len(g) < 200 {
		args = append(args, "-g", g, "--case-sensitive=false")
	}
	if s := q.Get("since"); s != "" && reSince.MatchString(s) {
		args = append(args, "--since", s)
	}
	cmd := exec.CommandContext(r.Context(), "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		httpErr(w, 500, "journalctl: "+err.Error())
		return
	}
	defer cmd.Wait()
	ch := make(chan any, 512)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			var e jEntry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			ts, _ := strconv.ParseInt(e.TS, 10, 64)
			ident := e.Ident
			if ident == "" {
				ident = e.Comm
			}
			if ident == "" {
				ident = strings.TrimSuffix(e.Unit, ".service")
			}
			prio := 6
			if e.Prio != "" {
				prio, _ = strconv.Atoi(e.Prio)
			}
			select {
			case ch <- []any{ts / 1000, prio, ident, trimLine(jMessage(e.Msg))}:
			case <-r.Context().Done():
				return
			}
		}
	}()
	lineStream(w, r, ch)
}

// ---------- timers & cron ----------

type timer struct {
	Unit      string `json:"unit"`
	Desc      string `json:"desc"`
	Next      string `json:"next"`
	Last      string `json:"last"`
	Activates string `json:"activates"`
	Active    string `json:"active"`
}

func listTimers(ctx context.Context) []timer {
	out, _ := systemctl(ctx, "list-units", "--type=timer", "--all", "--plain", "--no-legend")
	var names []string
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 && strings.HasSuffix(f[0], ".timer") {
			names = append(names, f[0])
		}
	}
	if len(names) == 0 {
		return nil
	}
	show, _ := systemctl(ctx, append([]string{"show", "-p", "Id,Description,NextElapseUSecRealtime,LastTriggerUSec,Unit,ActiveState"}, names...)...)
	var res []timer
	for _, block := range strings.Split(show, "\n\n") {
		var t timer
		for _, l := range strings.Split(block, "\n") {
			k, v, _ := strings.Cut(l, "=")
			switch k {
			case "Id":
				t.Unit = v
			case "Description":
				t.Desc = v
			case "NextElapseUSecRealtime":
				t.Next = v
			case "LastTriggerUSec":
				t.Last = v
			case "Unit":
				t.Activates = v
			case "ActiveState":
				t.Active = v
			}
		}
		if t.Unit != "" {
			res = append(res, t)
		}
	}
	return res
}

type cronJob struct {
	Sched  string `json:"sched"`
	User   string `json:"user"`
	Cmd    string `json:"cmd"`
	Source string `json:"source"`
}

func listCron() []cronJob {
	var jobs []cronJob
	parse := func(path, user string, hasUser bool) {
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || l[0] == '#' {
				continue
			}
			f := strings.Fields(l)
			if len(f) < 2 || (strings.Contains(f[0], "=") && !strings.HasPrefix(f[0], "@")) {
				continue
			}
			nSched := 5
			if strings.HasPrefix(f[0], "@") {
				nSched = 1
			}
			need := nSched + 1
			if hasUser {
				need++
			}
			if len(f) < need {
				continue
			}
			j := cronJob{Sched: strings.Join(f[:nSched], " "), User: user, Source: path}
			rest := f[nSched:]
			if hasUser {
				j.User, rest = rest[0], rest[1:]
			}
			j.Cmd = strings.Join(rest, " ")
			jobs = append(jobs, j)
		}
	}
	parse("/etc/crontab", "", true)
	if ents, err := os.ReadDir("/etc/cron.d"); err == nil {
		for _, e := range ents {
			if !strings.HasPrefix(e.Name(), ".") {
				parse("/etc/cron.d/"+e.Name(), "", true)
			}
		}
	}
	for _, dir := range []string{"/var/spool/cron/crontabs", "/var/spool/cron"} {
		if ents, err := os.ReadDir(dir); err == nil {
			for _, e := range ents {
				if !e.IsDir() {
					parse(dir+"/"+e.Name(), e.Name(), false)
				}
			}
		}
	}
	return jobs
}

func handleSchedule(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	writeJSON(w, r, map[string]any{"timers": listTimers(ctx), "cron": listCron()})
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Startup apps are plain systemd units named iseall-<name>.service. The first line holds the
// form values as JSON so the UI can round-trip them; everything else is a normal unit file.

const unitDir = "/etc/systemd/system"
const metaPrefix = "# iseall:"

type startupApp struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	WorkDir string   `json:"workdir"`
	User    string   `json:"user"`
	Restart string   `json:"restart"` // always | on-failure | no
	Env     []string `json:"env"`
	Delay   int      `json:"delay"`
	Network bool     `json:"network"`

	Active  string `json:"active,omitempty"`
	Sub     string `json:"sub,omitempty"`
	Enabled string `json:"enabled,omitempty"`
	PID     string `json:"pid,omitempty"`
	Since   string `json:"since,omitempty"`
}

var (
	reAppName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	reEnv     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.*$`)
)

func unitFor(name string) string { return "iseall-" + name + ".service" }

// systemd ExecStart quoting: double quotes with C escapes; $ and % are specifiers.
func sdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `$$`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}

func renderUnit(a startupApp) string {
	meta, _ := json.Marshal(startupApp{Name: a.Name, Command: a.Command, WorkDir: a.WorkDir, User: a.User, Restart: a.Restart, Env: a.Env, Delay: a.Delay, Network: a.Network})
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n# Managed by iseall — edit from the dashboard.\n\n[Unit]\nDescription=iseall: %s\n", metaPrefix, meta, a.Name)
	if a.Network {
		b.WriteString("Wants=network-online.target\nAfter=network-online.target\n")
	}
	b.WriteString("StartLimitIntervalSec=0\n\n[Service]\nType=simple\n")
	if a.User != "" && a.User != "root" {
		fmt.Fprintf(&b, "User=%s\n", a.User)
	}
	if a.WorkDir != "" {
		fmt.Fprintf(&b, "WorkingDirectory=%s\n", a.WorkDir)
	}
	for _, e := range a.Env {
		fmt.Fprintf(&b, "Environment=%s\n", sdQuote(e))
	}
	if a.Delay > 0 {
		fmt.Fprintf(&b, "ExecStartPre=/bin/sleep %d\n", a.Delay)
	}
	fmt.Fprintf(&b, "ExecStart=/bin/sh -c %s\n", sdQuote(a.Command))
	fmt.Fprintf(&b, "Restart=%s\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n", a.Restart)
	return b.String()
}

func readApp(path string) (startupApp, bool) {
	f, err := os.Open(path)
	if err != nil {
		return startupApp{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	if !sc.Scan() {
		return startupApp{}, false
	}
	line, found := strings.CutPrefix(sc.Text(), metaPrefix)
	var a startupApp
	if !found || json.Unmarshal([]byte(line), &a) != nil {
		return startupApp{}, false
	}
	return a, true
}

func listApps(ctx context.Context) []startupApp {
	paths, _ := filepath.Glob(filepath.Join(unitDir, "iseall-*.service"))
	apps := []startupApp{}
	var units []string
	for _, p := range paths {
		if a, ok := readApp(p); ok {
			apps = append(apps, a)
			units = append(units, unitFor(a.Name))
		}
	}
	if len(units) == 0 {
		return apps
	}
	show, _ := systemctl(ctx, append([]string{"show", "-p", "Id,ActiveState,SubState,UnitFileState,MainPID,ActiveEnterTimestamp"}, units...)...)
	st := map[string]map[string]string{}
	for _, block := range strings.Split(show, "\n\n") {
		m := map[string]string{}
		for _, l := range strings.Split(block, "\n") {
			if k, v, ok := strings.Cut(l, "="); ok {
				m[k] = v
			}
		}
		st[m["Id"]] = m
	}
	for i := range apps {
		if m := st[unitFor(apps[i].Name)]; m != nil {
			apps[i].Active, apps[i].Sub, apps[i].Enabled, apps[i].PID, apps[i].Since = m["ActiveState"], m["SubState"], m["UnitFileState"], m["MainPID"], m["ActiveEnterTimestamp"]
		}
	}
	return apps
}

var appMu sync.Mutex

func handleStartupSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		startupApp
		Original string `json:"original"`
		StartNow bool   `json:"startNow"`
	}
	if readJSON(r, &req) != nil {
		httpErr(w, 400, "bad request")
		return
	}
	a := req.startupApp
	a.Command = strings.TrimSpace(a.Command)
	switch {
	case !reAppName.MatchString(a.Name):
		httpErr(w, 400, "name: lowercase letters, digits and dashes (max 40)")
		return
	case a.Command == "" || strings.ContainsAny(a.Command, "\n\r\x00"):
		httpErr(w, 400, "command must be a single non-empty line")
		return
	case a.WorkDir != "" && (!filepath.IsAbs(a.WorkDir) || strings.ContainsAny(a.WorkDir, "\n\r")):
		httpErr(w, 400, "working directory must be an absolute path")
		return
	case a.User != "" && a.User != "root" && !userExists(a.User):
		httpErr(w, 400, "user does not exist: "+a.User)
		return
	case a.Delay < 0 || a.Delay > 3600:
		httpErr(w, 400, "delay must be 0–3600 seconds")
		return
	}
	if a.WorkDir != "" {
		if st, err := os.Stat(a.WorkDir); err != nil || !st.IsDir() {
			httpErr(w, 400, "working directory does not exist")
			return
		}
	}
	if a.Restart != "always" && a.Restart != "on-failure" && a.Restart != "no" {
		a.Restart = "on-failure"
	}
	for _, e := range a.Env {
		if !reEnv.MatchString(e) || strings.ContainsAny(e, "\n\r") {
			httpErr(w, 400, "bad environment entry: "+e)
			return
		}
	}
	appMu.Lock()
	defer appMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	path := filepath.Join(unitDir, unitFor(a.Name))
	if req.Original != a.Name {
		if _, err := os.Stat(path); err == nil {
			httpErr(w, 409, "a startup app with that name already exists")
			return
		}
	}
	if err := os.WriteFile(path+".tmp", []byte(renderUnit(a)), 0644); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	// renamed: retire the old unit
	if req.Original != "" && req.Original != a.Name && reAppName.MatchString(req.Original) {
		systemctl(ctx, "disable", "--now", unitFor(req.Original))
		os.Remove(filepath.Join(unitDir, unitFor(req.Original)))
	}
	if _, err := systemctl(ctx, "daemon-reload"); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	if _, err := systemctl(ctx, "enable", unitFor(a.Name)); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	if req.StartNow {
		if _, err := systemctl(ctx, "restart", unitFor(a.Name)); err != nil {
			httpErr(w, 500, "saved, but failed to start: "+err.Error())
			return
		}
	}
	ok(w, r)
}

func handleStartupDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !reAppName.MatchString(name) {
		httpErr(w, 400, "bad name")
		return
	}
	appMu.Lock()
	defer appMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	systemctl(ctx, "disable", "--now", unitFor(name))
	if err := os.Remove(filepath.Join(unitDir, unitFor(name))); err != nil && !os.IsNotExist(err) {
		httpErr(w, 500, err.Error())
		return
	}
	systemctl(ctx, "daemon-reload")
	ok(w, r)
}

// ---------- everything that runs at boot, in one place ----------

type autostart struct {
	Name   string `json:"name"`
	Exec   string `json:"exec"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
}

var analyzeCache = struct {
	sync.Mutex
	at    time.Time
	total string
	blame [][2]string
}{}

func bootAnalysis(ctx context.Context) (string, [][2]string) {
	analyzeCache.Lock()
	defer analyzeCache.Unlock()
	if time.Since(analyzeCache.at) < 10*time.Minute && analyzeCache.total != "" {
		return analyzeCache.total, analyzeCache.blame
	}
	if out, err := exec.CommandContext(ctx, "systemd-analyze", "time").Output(); err == nil {
		analyzeCache.total = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	}
	analyzeCache.blame = nil
	if out, err := exec.CommandContext(ctx, "systemd-analyze", "blame", "--no-pager").Output(); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			f := strings.Fields(l)
			if len(f) < 2 {
				continue
			}
			analyzeCache.blame = append(analyzeCache.blame, [2]string{strings.Join(f[:len(f)-1], " "), f[len(f)-1]})
			if len(analyzeCache.blame) >= 15 {
				break
			}
		}
	}
	analyzeCache.at = time.Now()
	return analyzeCache.total, analyzeCache.blame
}

func desktopEntries() []autostart {
	var out []autostart
	homes, _ := filepath.Glob("/home/*")
	homes = append(homes, "/root")
	dirs := []string{"/etc/xdg/autostart"}
	for _, h := range homes {
		dirs = append(dirs, h+"/.config/autostart")
	}
	for _, d := range dirs {
		files, _ := filepath.Glob(d + "/*.desktop")
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			a := autostart{Source: f, Kind: "desktop"}
			hidden := false
			for _, l := range strings.Split(string(b), "\n") {
				k, v, _ := strings.Cut(strings.TrimSpace(l), "=")
				switch k {
				case "Name":
					if a.Name == "" {
						a.Name = v
					}
				case "Exec":
					a.Exec = v
				case "Hidden":
					hidden = v == "true"
				}
			}
			if !hidden {
				out = append(out, a)
			}
		}
	}
	// Raspberry Pi OS (Wayland): labwc and wayfire autostart
	lab := []string{"/etc/xdg/labwc/autostart"}
	for _, h := range homes {
		lab = append(lab, h+"/.config/labwc/autostart")
	}
	for _, f := range lab {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" && l[0] != '#' {
				out = append(out, autostart{Name: strings.Fields(l)[0], Exec: l, Source: f, Kind: "labwc"})
			}
		}
	}
	for _, h := range homes {
		f := h + "/.config/wayfire.ini"
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		in := false
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "[") {
				in = l == "[autostart]"
				continue
			}
			if k, v, ok := strings.Cut(l, "="); in && ok && !strings.HasPrefix(l, "#") {
				out = append(out, autostart{Name: strings.TrimSpace(k), Exec: strings.TrimSpace(v), Source: f, Kind: "wayfire"})
			}
		}
	}
	return out
}

func handleBoot(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	res := map[string]any{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	set := func(k string, v any) { mu.Lock(); res[k] = v; mu.Unlock() }
	wg.Add(3)
	go func() { defer wg.Done(); set("apps", listApps(ctx)) }()
	go func() {
		defer wg.Done()
		total, blame := bootAnalysis(ctx)
		set("bootTime", total)
		set("blame", blame)
	}()
	go func() {
		defer wg.Done()
		out, _ := systemctl(ctx, "list-unit-files", "--type=service", "--state=enabled", "--plain", "--no-legend")
		var en []string
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) > 0 && !strings.HasPrefix(f[0], "iseall-") {
				en = append(en, f[0])
			}
		}
		set("enabled", en)
	}()
	wg.Wait()

	type ctrBoot struct {
		Name   string `json:"name"`
		Policy string `json:"policy"`
		State  string `json:"state"`
		ID     string `json:"id"`
	}
	var cb []ctrBoot
	if list, err := containers(2 * time.Second); err == nil {
		for _, c := range list {
			cb = append(cb, ctrBoot{Name: c.name(), Policy: restartPolicy(c.ID), State: c.State, ID: c.ID})
		}
	}
	res["containers"] = cb
	var reboot []cronJob
	for _, j := range listCron() {
		if j.Sched == "@reboot" {
			reboot = append(reboot, j)
		}
	}
	res["cronReboot"] = reboot
	res["autostart"] = desktopEntries()
	if b, err := os.ReadFile("/etc/rc.local"); err == nil {
		var lines []string
		for _, l := range strings.Split(string(b), "\n") {
			if t := strings.TrimSpace(l); t != "" && t[0] != '#' && t != "exit 0" {
				lines = append(lines, t)
			}
		}
		res["rcLocal"] = lines
	}
	writeJSON(w, r, res)
}

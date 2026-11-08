package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// nginx management: status, sites (Debian sites-available/enabled or RHEL-style conf.d),
// a config editor that refuses to keep a change `nginx -t` rejects, access-log stats,
// log tailing and certbot.

var nginxMu sync.Mutex // serialises every config change + test + reload

type ngxEnv struct {
	Bin      string
	Version  string
	ConfPath string
	ConfDir  string
	Layout   string // "debian" | "confd"
}

var reNgxConf = regexp.MustCompile(`--conf-path=(\S+)`)

func nginxEnv(ctx context.Context) (*ngxEnv, error) {
	bin, err := exec.LookPath("nginx")
	if err != nil {
		for _, p := range []string{"/usr/sbin/nginx", "/usr/local/sbin/nginx", "/usr/local/nginx/sbin/nginx"} {
			if st, e := os.Stat(p); e == nil && !st.IsDir() {
				bin, err = p, nil
				break
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("nginx isn't installed")
	}
	e := &ngxEnv{Bin: bin, ConfPath: "/etc/nginx/nginx.conf"}
	out, _ := exec.CommandContext(ctx, bin, "-V").CombinedOutput()
	for _, l := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(l, "nginx version: "); ok {
			e.Version = strings.TrimPrefix(strings.TrimSpace(v), "nginx/")
		}
	}
	if m := reNgxConf.FindSubmatch(out); m != nil {
		e.ConfPath = string(m[1])
	}
	e.ConfDir = filepath.Dir(e.ConfPath)
	e.Layout = "confd"
	if st, err := os.Stat(filepath.Join(e.ConfDir, "sites-available")); err == nil && st.IsDir() {
		e.Layout = "debian"
	}
	return e, nil
}

func (e *ngxEnv) test(ctx context.Context) (bool, string) {
	out, err := exec.CommandContext(ctx, e.Bin, "-t", "-q").CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = err.Error()
		}
		return false, s
	}
	return true, s
}

func hasNginxUnit(ctx context.Context) bool {
	out, _ := systemctl(ctx, "show", "nginx.service", "-p", "LoadState", "--value")
	return strings.TrimSpace(out) == "loaded"
}

func nginxRunning(ctx context.Context) bool {
	if hasNginxUnit(ctx) {
		out, _ := systemctl(ctx, "is-active", "nginx.service")
		return strings.TrimSpace(out) == "active"
	}
	return exec.CommandContext(ctx, "pgrep", "-x", "nginx").Run() == nil
}

// reload only if running — a stopped nginx stays stopped
func (e *ngxEnv) reloadIfRunning(ctx context.Context) error {
	if !nginxRunning(ctx) {
		return nil
	}
	return e.ctl(ctx, "reload")
}

func (e *ngxEnv) ctl(ctx context.Context, action string) error {
	if hasNginxUnit(ctx) {
		_, err := systemctl(ctx, action, "nginx.service")
		return err
	}
	var args []string
	switch action {
	case "reload":
		args = []string{"-s", "reload"}
	case "stop":
		args = []string{"-s", "quit"}
	case "restart":
		exec.CommandContext(ctx, e.Bin, "-s", "quit").Run()
		time.Sleep(time.Second)
	}
	out, err := exec.CommandContext(ctx, e.Bin, args...).CombinedOutput()
	if err != nil {
		return &cmdErr{strings.TrimSpace(string(out) + " " + err.Error())}
	}
	return nil
}

// ---------- tiny config parser: enough to describe server blocks ----------

func ngxTokens(src string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '#':
			flush()
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			flush()
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			toks = append(toks, src[i+1:min(j, len(src))])
			i = j
		case c == '{' || c == '}' || c == ';':
			flush()
			toks = append(toks, string(c))
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return toks
}

type ngxServer struct {
	Names   []string `json:"names"`
	Listen  []string `json:"listen"`
	SSL     bool     `json:"ssl"`
	Root    string   `json:"root,omitempty"`
	Proxies []string `json:"proxies,omitempty"`
	Return  string   `json:"return,omitempty"`
}

func ngxServers(src string) []ngxServer {
	toks := ngxTokens(src)
	res := []ngxServer{}
	var stack, stmt []string
	cur := -1 // index into res of the server block being read
	for _, t := range toks {
		switch t {
		case "{":
			name := ""
			if len(stmt) > 0 {
				name = stmt[0]
			}
			// "server" inside upstream/stream blocks isn't an http vhost
			if name == "server" && cur < 0 && !contains(stack, "upstream") && !contains(stack, "stream") {
				res = append(res, ngxServer{Names: []string{}, Listen: []string{}})
				cur = len(res) - 1
				name = "server*"
			}
			stack = append(stack, name)
			stmt = nil
		case "}":
			if len(stack) > 0 {
				if stack[len(stack)-1] == "server*" {
					cur = -1
				}
				stack = stack[:len(stack)-1]
			}
			stmt = nil
		case ";":
			if cur >= 0 && len(stmt) > 0 {
				s, args := &res[cur], stmt[1:]
				switch stmt[0] {
				case "server_name":
					s.Names = append(s.Names, args...)
				case "listen":
					if len(args) > 0 {
						s.Listen = append(s.Listen, args[0])
						if contains(args, "ssl") || contains(args, "quic") {
							s.SSL = true
						}
					}
				case "ssl_certificate":
					s.SSL = true
				case "root":
					if len(args) > 0 && s.Root == "" {
						s.Root = args[0]
					}
				case "proxy_pass", "fastcgi_pass", "uwsgi_pass", "grpc_pass":
					if len(args) > 0 && !contains(s.Proxies, args[0]) {
						s.Proxies = append(s.Proxies, args[0])
					}
				case "return":
					if s.Return == "" && len(stack) > 0 && stack[len(stack)-1] == "server*" {
						s.Return = strings.Join(args, " ")
					}
				}
			}
			stmt = nil
		default:
			stmt = append(stmt, t)
		}
	}
	return res
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

// first occurrence of a directive (at statement start) → its first argument
func ngxDirective(src, name string) string {
	toks := ngxTokens(src)
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == name && (i == 0 || toks[i-1] == ";" || toks[i-1] == "{" || toks[i-1] == "}") {
			return toks[i+1]
		}
	}
	return ""
}

// ---------- sites ----------

type ngxSite struct {
	ID      string      `json:"id"` // path relative to the conf dir
	Name    string      `json:"name"`
	Enabled bool        `json:"enabled"`
	Kind    string      `json:"kind"` // site | conf
	Size    int64       `json:"size"`
	MTime   int64       `json:"mtime"`
	Servers []ngxServer `json:"servers"`
}

func (e *ngxEnv) sites() []ngxSite {
	out := []ngxSite{}
	add := func(rel string, enabled bool, kind string) {
		p := filepath.Join(e.ConfDir, rel)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			return
		}
		s := ngxSite{ID: rel, Name: filepath.Base(rel), Enabled: enabled, Kind: kind, Size: st.Size(), MTime: st.ModTime().Unix(), Servers: []ngxServer{}}
		if st.Size() < 1<<20 {
			if b, err := os.ReadFile(p); err == nil {
				s.Servers = ngxServers(string(b))
			}
		}
		out = append(out, s)
	}
	if e.Layout == "debian" {
		avail := filepath.Join(e.ConfDir, "sites-available")
		en := filepath.Join(e.ConfDir, "sites-enabled")
		enabled := map[string]bool{}
		if ents, err := os.ReadDir(en); err == nil {
			for _, d := range ents {
				p := filepath.Join(en, d.Name())
				if d.Type()&os.ModeSymlink != 0 {
					if t, err := filepath.EvalSymlinks(p); err == nil && filepath.Dir(t) == avail {
						enabled[filepath.Base(t)] = true
						continue
					}
				}
				if d.Type().IsRegular() { // a real file living in sites-enabled
					add("sites-enabled/"+d.Name(), true, "site")
				}
			}
		}
		if ents, err := os.ReadDir(avail); err == nil {
			for _, d := range ents {
				if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
					add("sites-available/"+d.Name(), enabled[d.Name()], "site")
				}
			}
		}
	}
	if ents, err := os.ReadDir(filepath.Join(e.ConfDir, "conf.d")); err == nil {
		for _, d := range ents {
			n := d.Name()
			if d.IsDir() || strings.HasPrefix(n, ".") {
				continue
			}
			if strings.HasSuffix(n, ".conf") {
				add("conf.d/"+n, true, "conf")
			} else if strings.HasSuffix(n, ".conf.disabled") {
				add("conf.d/"+n, false, "conf")
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == "site"
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// resolve a client-supplied config path; it must stay inside the nginx conf dir
func (e *ngxEnv) confFile(rel string) (string, error) {
	if rel == "" || strings.Contains(rel, "\x00") {
		return "", fmt.Errorf("bad path")
	}
	p := rel
	if !filepath.IsAbs(p) {
		p = filepath.Join(e.ConfDir, rel)
	}
	p = filepath.Clean(p)
	root, err := filepath.EvalSymlinks(e.ConfDir)
	if err != nil {
		root = e.ConfDir
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		real = p
		if d, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
			real = filepath.Join(d, filepath.Base(p))
		}
	}
	if real == root || !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return "", fmt.Errorf("only files under %s can be edited", e.ConfDir)
	}
	return real, nil
}

// ---------- access log stats ----------

var reAccess = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(?:(\S+) (\S+)[^"]*|[^"]*)" (\d{3}) (\d+|-)`)

type kvCount struct {
	K string `json:"k"`
	N int    `json:"n"`
}

type ngxStats struct {
	File    string    `json:"file"`
	Lines   int       `json:"lines"`
	From    int64     `json:"from"`
	To      int64     `json:"to"`
	Bytes   int64     `json:"bytes"`
	Status  [6]int    `json:"status"` // index 1..5 = 1xx..5xx
	Paths   []kvCount `json:"paths"`
	IPs     []kvCount `json:"ips"`
	PerMin  []int     `json:"perMin"` // last 60 minutes, oldest first
	LastErr []string  `json:"lastErr"`
}

func topN(m map[string]int, n int) []kvCount {
	res := make([]kvCount, 0, len(m))
	for k, v := range m {
		res = append(res, kvCount{k, v})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].N > res[j].N || (res[i].N == res[j].N && res[i].K < res[j].K) })
	if len(res) > n {
		res = res[:n]
	}
	return res
}

func tailBytes(path string, n int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	off := max(st.Size()-n, 0)
	b, _ := io.ReadAll(io.NewSectionReader(f, off, st.Size()-off))
	if off > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return b
}

func accessStats(accessLog, errorLog string) *ngxStats {
	s := &ngxStats{File: accessLog, PerMin: make([]int, 60), Paths: []kvCount{}, IPs: []kvCount{}, LastErr: []string{}}
	paths, ips := map[string]int{}, map[string]int{}
	now := time.Now()
	sc := bufio.NewScanner(bytes.NewReader(tailBytes(accessLog, 1<<20)))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		m := reAccess.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		s.Lines++
		if t, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[2]); err == nil {
			if s.From == 0 {
				s.From = t.Unix()
			}
			s.To = t.Unix()
			if ago := int(now.Sub(t) / time.Minute); ago >= 0 && ago < 60 {
				s.PerMin[59-ago]++
			}
		}
		if c := m[5][0] - '0'; c >= 1 && c <= 5 {
			s.Status[c]++
		}
		if b, err := strconv.ParseInt(m[6], 10, 64); err == nil {
			s.Bytes += b
		}
		ips[m[1]]++
		if p := m[4]; p != "" {
			if i := strings.IndexByte(p, '?'); i >= 0 {
				p = p[:i]
			}
			if len(p) > 120 {
				p = p[:120] + "…"
			}
			paths[p]++
		}
	}
	s.Paths, s.IPs = topN(paths, 8), topN(ips, 8)
	lines := strings.Split(strings.TrimSpace(string(tailBytes(errorLog, 16<<10))), "\n")
	for i := len(lines) - 1; i >= 0 && len(s.LastErr) < 5; i-- {
		l := strings.TrimSpace(lines[i])
		for _, lv := range []string{"[error]", "[crit]", "[alert]", "[emerg]"} {
			if strings.Contains(l, lv) {
				s.LastErr = append(s.LastErr, trimLine(l))
				break
			}
		}
	}
	return s
}

func (e *ngxEnv) logPaths() (string, string) {
	acc, errl := "/var/log/nginx/access.log", "/var/log/nginx/error.log"
	if b, err := os.ReadFile(e.ConfPath); err == nil {
		src := string(b)
		if v := ngxDirective(src, "access_log"); strings.HasPrefix(v, "/") {
			acc = v
		}
		if v := ngxDirective(src, "error_log"); strings.HasPrefix(v, "/") {
			errl = v
		}
	}
	return acc, errl
}

// ---------- handlers ----------

func handleNginx(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	e, err := nginxEnv(ctx)
	if err != nil {
		writeJSON(w, r, map[string]any{"installed": false, "reason": err.Error()})
		return
	}
	svc := map[string]string{}
	unit := hasNginxUnit(ctx)
	if unit {
		show, _ := systemctl(ctx, "show", "nginx.service", "-p", "ActiveState,SubState,UnitFileState,MainPID,ActiveEnterTimestamp,MemoryCurrent")
		for _, l := range strings.Split(show, "\n") {
			if k, v, ok := strings.Cut(l, "="); ok {
				svc[k] = v
			}
		}
	} else if nginxRunning(ctx) {
		svc["ActiveState"], svc["SubState"] = "active", "running"
	} else {
		svc["ActiveState"], svc["SubState"] = "inactive", "dead"
	}
	workers := 0
	if out, err := exec.CommandContext(ctx, "pgrep", "-c", "-f", "^nginx: worker").Output(); err == nil {
		workers, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}
	okT, testOut := e.test(ctx)
	acc, errl := e.logPaths()
	_, certbotErr := exec.LookPath("certbot")
	writeJSON(w, r, map[string]any{
		"installed": true, "version": e.Version, "bin": e.Bin, "conf": e.ConfPath, "confDir": e.ConfDir, "layout": e.Layout,
		"unit": unit, "service": svc, "workers": workers,
		"test":    map[string]any{"ok": okT, "out": testOut},
		"sites":   e.sites(),
		"logs":    map[string]string{"access": acc, "error": errl},
		"stats":   accessStats(acc, errl),
		"certbot": certbotErr == nil,
	})
}

func handleNginxCtl(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	e, err := nginxEnv(ctx)
	if err != nil {
		httpErr(w, 404, err.Error())
		return
	}
	nginxMu.Lock()
	defer nginxMu.Unlock()
	switch action {
	case "test":
		okT, out := e.test(ctx)
		writeJSON(w, r, map[string]any{"ok": okT, "out": out})
		return
	case "reload", "restart", "start":
		// with a broken config, restart would take a running nginx down — refuse up front
		if okT, out := e.test(ctx); !okT {
			httpErr(w, 422, "config test failed — nothing was changed:\n"+out)
			return
		}
	case "stop":
	case "enable", "disable":
		if _, err := systemctl(ctx, action, "nginx.service"); err != nil {
			httpErr(w, 500, err.Error())
			return
		}
		ok(w, r)
		return
	default:
		httpErr(w, 400, "unknown action")
		return
	}
	if err := e.ctl(ctx, action); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	ok(w, r)
}

func handleNginxFileGet(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	e, err := nginxEnv(ctx)
	if err != nil {
		httpErr(w, 404, err.Error())
		return
	}
	p, err := e.confFile(r.URL.Query().Get("path"))
	if err != nil {
		httpErr(w, 400, err.Error())
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		httpErr(w, 404, err.Error())
		return
	}
	if st.IsDir() || st.Size() > 1<<20 {
		httpErr(w, 400, "not an editable file")
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	writeJSON(w, r, map[string]any{"path": p, "content": string(b), "mtime": st.ModTime().UnixNano()})
}

// writeTested replaces a file, runs `nginx -t`, and puts the old contents back if the test fails.
func (e *ngxEnv) writeTested(ctx context.Context, p string, content []byte) (string, error) {
	old, readErr := os.ReadFile(p)
	if readErr != nil {
		return "", readErr
	}
	mode := os.FileMode(0o644)
	var mtime time.Time
	if st, err := os.Stat(p); err == nil {
		mode, mtime = st.Mode().Perm(), st.ModTime()
	}
	// write in place (not rename) so ownership, hard links and SELinux labels survive
	if err := os.WriteFile(p, content, mode); err != nil {
		return "", err
	}
	if okT, out := e.test(ctx); !okT {
		os.WriteFile(p, old, mode)
		os.Chtimes(p, mtime, mtime) // so the editor's next save isn't flagged as a conflict
		return out, fmt.Errorf("config test failed — your change was not kept")
	}
	return "", nil
}

func handleNginxFileSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		MTime   int64  `json:"mtime"`
		Reload  bool   `json:"reload"`
	}
	if readJSON(r, &req) != nil {
		httpErr(w, 400, "bad request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	e, err := nginxEnv(ctx)
	if err != nil {
		httpErr(w, 404, err.Error())
		return
	}
	p, err := e.confFile(req.Path)
	if err != nil {
		httpErr(w, 400, err.Error())
		return
	}
	nginxMu.Lock()
	defer nginxMu.Unlock()
	if st, err := os.Stat(p); err == nil && req.MTime != 0 && st.ModTime().UnixNano() != req.MTime {
		httpErr(w, 409, "the file changed on disk since you opened it — reopen it and reapply your edit")
		return
	}
	if out, err := e.writeTested(ctx, p, []byte(req.Content)); err != nil {
		httpErr(w, 422, strings.TrimSpace(err.Error()+"\n"+out))
		return
	}
	if req.Reload {
		if err := e.reloadIfRunning(ctx); err != nil {
			httpErr(w, 500, "saved, but reload failed: "+err.Error())
			return
		}
	}
	var mt int64
	if st, err := os.Stat(p); err == nil {
		mt = st.ModTime().UnixNano()
	}
	writeJSON(w, r, map[string]any{"ok": true, "mtime": mt})
}

var reSiteName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,80}$`)

func handleNginxSite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action  string `json:"action"` // enable | disable | delete | create
		ID      string `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
		Enable  bool   `json:"enable"`
	}
	if readJSON(r, &req) != nil {
		httpErr(w, 400, "bad request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	e, err := nginxEnv(ctx)
	if err != nil {
		httpErr(w, 404, err.Error())
		return
	}
	nginxMu.Lock()
	defer nginxMu.Unlock()

	// every change is followed by a config test; undo runs if the test fails
	commit := func(undo func()) bool {
		if okT, out := e.test(ctx); !okT {
			undo()
			httpErr(w, 422, "config test failed — change undone:\n"+out)
			return false
		}
		if err := e.reloadIfRunning(ctx); err != nil {
			httpErr(w, 500, "changed, but reload failed: "+err.Error())
			return false
		}
		return true
	}

	if req.Action == "create" {
		name := strings.TrimSuffix(req.Name, ".conf")
		if !reSiteName.MatchString(name) {
			httpErr(w, 400, "names may use letters, digits, dot, dash and underscore")
			return
		}
		var rel string
		if e.Layout == "debian" {
			rel = "sites-available/" + name
		} else {
			rel = "conf.d/" + name + ".conf"
		}
		p := filepath.Join(e.ConfDir, rel)
		for _, q := range []string{p, p + ".disabled", filepath.Join(e.ConfDir, "sites-enabled", name)} {
			if _, err := os.Lstat(q); err == nil {
				httpErr(w, 409, "a site called "+name+" already exists")
				return
			}
		}
		if e.Layout != "debian" && !req.Enable {
			p += ".disabled"
			rel += ".disabled"
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(req.Content), 0o644); err != nil {
			httpErr(w, 500, err.Error())
			return
		}
		link := ""
		if e.Layout == "debian" && req.Enable {
			os.MkdirAll(filepath.Join(e.ConfDir, "sites-enabled"), 0o755)
			link = filepath.Join(e.ConfDir, "sites-enabled", name)
			if err := os.Symlink(p, link); err != nil {
				os.Remove(p)
				httpErr(w, 500, err.Error())
				return
			}
		}
		if !commit(func() {
			if link != "" {
				os.Remove(link)
			}
			os.Remove(p)
		}) {
			return
		}
		writeJSON(w, r, map[string]any{"ok": true, "id": rel})
		return
	}

	rel := filepath.ToSlash(filepath.Clean(req.ID))
	dir, base := filepath.Split(rel)
	if !reSiteName.MatchString(base) || (dir != "sites-available/" && dir != "sites-enabled/" && dir != "conf.d/") {
		httpErr(w, 400, "bad site")
		return
	}
	p := filepath.Join(e.ConfDir, rel)
	if _, err := os.Lstat(p); err != nil {
		httpErr(w, 404, "site not found")
		return
	}
	enLink := filepath.Join(e.ConfDir, "sites-enabled", base)
	enabled := false
	switch dir {
	case "sites-enabled/":
		enabled = true
	case "conf.d/":
		enabled = strings.HasSuffix(base, ".conf")
	default:
		t, err := filepath.EvalSymlinks(enLink)
		real, _ := filepath.EvalSymlinks(p)
		enabled = err == nil && t == real
	}

	switch req.Action {
	case "enable":
		if enabled {
			ok(w, r)
			return
		}
		if dir == "conf.d/" {
			np := strings.TrimSuffix(p, ".disabled")
			if np == p || !strings.HasSuffix(np, ".conf") {
				httpErr(w, 400, "only *.conf.disabled files can be enabled")
				return
			}
			if _, err := os.Stat(np); err == nil {
				httpErr(w, 409, filepath.Base(np)+" already exists")
				return
			}
			if err := os.Rename(p, np); err != nil {
				httpErr(w, 500, err.Error())
				return
			}
			if commit(func() { os.Rename(np, p) }) {
				writeJSON(w, r, map[string]any{"ok": true, "id": "conf.d/" + filepath.Base(np)})
			}
			return
		}
		if _, err := os.Lstat(enLink); err == nil {
			httpErr(w, 409, "sites-enabled/"+base+" already exists")
			return
		}
		os.MkdirAll(filepath.Dir(enLink), 0o755)
		if err := os.Symlink(p, enLink); err != nil {
			httpErr(w, 500, err.Error())
			return
		}
		if commit(func() { os.Remove(enLink) }) {
			ok(w, r)
		}
	case "disable":
		if !enabled {
			ok(w, r)
			return
		}
		switch dir {
		case "conf.d/":
			np := p + ".disabled"
			if err := os.Rename(p, np); err != nil {
				httpErr(w, 500, err.Error())
				return
			}
			if commit(func() { os.Rename(np, p) }) {
				writeJSON(w, r, map[string]any{"ok": true, "id": "conf.d/" + filepath.Base(np)})
			}
		case "sites-enabled/": // a real file in sites-enabled: move it to sites-available
			np := filepath.Join(e.ConfDir, "sites-available", base)
			if _, err := os.Lstat(np); err == nil {
				httpErr(w, 409, "sites-available/"+base+" already exists")
				return
			}
			os.MkdirAll(filepath.Dir(np), 0o755)
			if err := os.Rename(p, np); err != nil {
				httpErr(w, 500, err.Error())
				return
			}
			if commit(func() { os.Rename(np, p) }) {
				writeJSON(w, r, map[string]any{"ok": true, "id": "sites-available/" + base})
			}
		default:
			if err := os.Remove(enLink); err != nil {
				httpErr(w, 500, err.Error())
				return
			}
			if commit(func() { os.Symlink(p, enLink) }) {
				ok(w, r)
			}
		}
	case "delete":
		if enabled {
			httpErr(w, 409, "disable the site before deleting it")
			return
		}
		if err := os.Remove(p); err != nil {
			httpErr(w, 500, err.Error())
			return
		}
		ok(w, r)
	default:
		httpErr(w, 400, "unknown action")
	}
}

func handleNginxLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	e, err := nginxEnv(ctx)
	cancel()
	if err != nil {
		httpErr(w, 404, err.Error())
		return
	}
	acc, errl := e.logPaths()
	path := acc
	if q.Get("kind") == "error" {
		path = errl
	}
	if _, err := os.Stat(path); err != nil {
		httpErr(w, 404, "log not found: "+path)
		return
	}
	n, _ := strconv.Atoi(q.Get("n"))
	if n <= 0 || n > 5000 {
		n = 400
	}
	cmd := exec.CommandContext(r.Context(), "tail", "-n", strconv.Itoa(n), "-F", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		httpErr(w, 500, "tail: "+err.Error())
		return
	}
	defer cmd.Wait()
	ch := make(chan any, 512)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			select {
			case ch <- trimLine(sc.Text()):
			case <-r.Context().Done():
				return
			}
		}
	}()
	lineStream(w, r, ch)
}

var reDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)
var reEmail = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func handleNginxCertbot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domains []string `json:"domains"`
		Email   string   `json:"email"`
	}
	if readJSON(r, &req) != nil || len(req.Domains) == 0 || len(req.Domains) > 50 {
		httpErr(w, 400, "pick at least one domain")
		return
	}
	args := []string{"--nginx", "--non-interactive", "--agree-tos", "--redirect", "--keep-until-expiring"}
	for _, d := range req.Domains {
		if !reDomain.MatchString(d) {
			httpErr(w, 400, "not a public domain name: "+d)
			return
		}
		args = append(args, "-d", d)
	}
	if req.Email == "" {
		args = append(args, "--register-unsafely-without-email")
	} else if reEmail.MatchString(req.Email) {
		args = append(args, "-m", req.Email)
	} else {
		httpErr(w, 400, "bad email address")
		return
	}
	if _, err := exec.LookPath("certbot"); err != nil {
		httpErr(w, 404, "certbot isn't installed")
		return
	}
	nginxMu.Lock()
	defer nginxMu.Unlock()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)
	fw := flushWriter{w, fl}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	fmt.Fprintf(fw, "$ certbot %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "certbot", args...)
	cmd.Stdout, cmd.Stderr = fw, fw
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(fw, "\n✗ %v\n", err)
		return
	}
	fmt.Fprint(fw, "\n✓ done\n")
}

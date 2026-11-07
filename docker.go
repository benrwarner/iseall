package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const dockerSock = "/var/run/docker.sock"

var dockerHTTP = &http.Client{Transport: &http.Transport{
	DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", dockerSock)
	},
	MaxIdleConns:    4,
	IdleConnTimeout: 30 * time.Second,
}}

func dockerReq(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := dockerHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var e struct{ Message string }
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		json.Unmarshal(b, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(b))
		}
		return nil, fmt.Errorf("%s", e.Message)
	}
	return resp, nil
}

func dockerJSON(ctx context.Context, method, path string, body, out any) error {
	resp, err := dockerReq(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type dPort struct {
	IP          string
	PrivatePort int
	PublicPort  int
	Type        string
}

type dContainer struct {
	ID      string `json:"Id"`
	Names   []string
	Image   string
	ImageID string
	State   string
	Status  string
	Created int64
	Ports   []dPort
	Labels  map[string]string
}

func (c dContainer) name() string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return c.ID[:12]
}

// ---------- shared, short-lived container list cache ----------

var dcache = struct {
	sync.Mutex
	list []dContainer
	at   time.Time
	err  error
}{}

func containers(maxAge time.Duration) ([]dContainer, error) {
	dcache.Lock()
	defer dcache.Unlock()
	if time.Since(dcache.at) < maxAge {
		return dcache.list, dcache.err
	}
	if _, err := os.Stat(dockerSock); err != nil {
		dcache.list, dcache.err, dcache.at = nil, err, time.Now()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var list []dContainer
	err := dockerJSON(ctx, "GET", "/containers/json?all=1", nil, &list)
	dcache.list, dcache.err, dcache.at = list, err, time.Now()
	return list, err
}

func invalidateContainers() {
	dcache.Lock()
	dcache.at = time.Time{}
	dcache.Unlock()
}

func ctrNames() map[string]string {
	list, _ := containers(5 * time.Second)
	m := make(map[string]string, len(list))
	for _, c := range list {
		m[c.ID] = c.name()
	}
	return m
}

func dockerPublished() map[string]string {
	list, _ := containers(5 * time.Second)
	m := map[string]string{}
	for _, c := range list {
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				m[p.Type+":"+strconv.Itoa(p.PublicPort)] = c.name()
			}
		}
	}
	return m
}

type dSummary struct {
	Running int      `json:"running"`
	Total   int      `json:"total"`
	Bad     []string `json:"bad"`
}

var reExitCode = regexp.MustCompile(`^Exited \((\d+)\)`)

func dockerSummary() *dSummary {
	list, err := containers(4 * time.Second)
	if err != nil {
		return nil
	}
	s := &dSummary{Total: len(list), Bad: []string{}}
	for _, c := range list {
		switch {
		case c.State == "running" && strings.Contains(c.Status, "(unhealthy)"):
			s.Bad = append(s.Bad, c.name()+" is unhealthy")
		case c.State == "restarting":
			s.Bad = append(s.Bad, c.name()+" is restart-looping")
		case c.State == "exited":
			// 137/143 are SIGKILL/SIGTERM — i.e. somebody stopped it on purpose
			if m := reExitCode.FindStringSubmatch(c.Status); m != nil && m[1] != "0" && m[1] != "137" && m[1] != "143" {
				s.Bad = append(s.Bad, c.name()+" exited with code "+m[1])
			}
		}
		if c.State == "running" {
			s.Running++
		}
	}
	return s
}

// ---------- cgroup-based stats: far cheaper than the docker stats API ----------

type cgStat struct {
	usage uint64 // CPU ns
	at    time.Time
}

var cstats = struct {
	sync.Mutex
	prev  map[string]cgStat
	paths map[string]string
}{prev: map[string]cgStat{}, paths: map[string]string{}}

func cgroupDir(id string) string {
	if p, ok := cstats.paths[id]; ok {
		return p
	}
	for _, p := range []string{
		"/sys/fs/cgroup/system.slice/docker-" + id + ".scope",
		"/sys/fs/cgroup/docker/" + id,
		"/sys/fs/cgroup/cpuacct/docker/" + id,
		"/sys/fs/cgroup/cpu,cpuacct/docker/" + id,
		"/sys/fs/cgroup/cpu,cpuacct/system.slice/docker-" + id + ".scope",
	} {
		if _, err := os.Stat(p); err == nil {
			cstats.paths[id] = p
			return p
		}
	}
	return ""
}

func readCgroupStats(dir string) (cpuNs, mem, limit uint64) {
	if b, err := os.ReadFile(dir + "/cpu.stat"); err == nil { // cgroup v2
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "usage_usec "); ok {
				u, _ := strconv.ParseUint(v, 10, 64)
				cpuNs = u * 1000
			}
		}
		mem, _ = strconv.ParseUint(readTrim(dir+"/memory.current"), 10, 64)
		if sb, err := os.ReadFile(dir + "/memory.stat"); err == nil {
			for _, l := range strings.Split(string(sb), "\n") {
				if v, ok := strings.CutPrefix(l, "inactive_file "); ok {
					if inact, _ := strconv.ParseUint(v, 10, 64); inact < mem {
						mem -= inact
					}
				}
			}
		}
		limit, _ = strconv.ParseUint(readTrim(dir+"/memory.max"), 10, 64)
		return
	}
	cpuNs, _ = strconv.ParseUint(readTrim(dir+"/cpuacct.usage"), 10, 64) // cgroup v1
	mdir := strings.Replace(strings.Replace(dir, "cpu,cpuacct", "memory", 1), "cpuacct", "memory", 1)
	mem, _ = strconv.ParseUint(readTrim(mdir+"/memory.usage_in_bytes"), 10, 64)
	limit, _ = strconv.ParseUint(readTrim(mdir+"/memory.limit_in_bytes"), 10, 64)
	if limit > 1<<60 {
		limit = 0
	}
	return
}

// ---------- restart policy cache (needs inspect, which is per-container) ----------

var policies = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

func restartPolicy(id string) string {
	policies.Lock()
	p, ok := policies.m[id]
	policies.Unlock()
	if ok {
		return p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ins struct {
		HostConfig struct{ RestartPolicy struct{ Name string } }
	}
	if dockerJSON(ctx, "GET", "/containers/"+id+"/json", nil, &ins) != nil {
		return ""
	}
	p = ins.HostConfig.RestartPolicy.Name
	if p == "" {
		p = "no"
	}
	policies.Lock()
	policies.m[id] = p
	policies.Unlock()
	return p
}

type ctrOut struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Image    string   `json:"image"`
	State    string   `json:"state"`
	Status   string   `json:"status"`
	Created  int64    `json:"created"`
	Ports    []string `json:"ports"`
	Project  string   `json:"project,omitempty"`
	Service  string   `json:"service,omitempty"`
	Health   string   `json:"health,omitempty"`
	CPU      float64  `json:"cpu"`
	Mem      uint64   `json:"mem"`
	MemLimit uint64   `json:"memLimit"`
	Policy   string   `json:"policy"`
}

func handleDocker(w http.ResponseWriter, r *http.Request) {
	list, err := containers(time.Second)
	if err != nil {
		writeJSON(w, r, map[string]any{"available": false, "error": err.Error()})
		return
	}
	out := make([]ctrOut, len(list))
	var wg sync.WaitGroup
	now := time.Now()
	cstats.Lock()
	live := map[string]bool{}
	for i, c := range list {
		o := ctrOut{ID: c.ID, Name: c.name(), Image: c.Image, State: c.State, Status: c.Status, Created: c.Created,
			Project: c.Labels["com.docker.compose.project"], Service: c.Labels["com.docker.compose.service"]}
		switch {
		case strings.Contains(c.Status, "(healthy)"):
			o.Health = "healthy"
		case strings.Contains(c.Status, "(unhealthy)"):
			o.Health = "unhealthy"
		case strings.Contains(c.Status, "(health: starting)"):
			o.Health = "starting"
		}
		seen := map[string]bool{}
		for _, p := range c.Ports {
			s := strconv.Itoa(p.PrivatePort) + "/" + p.Type
			if p.PublicPort > 0 {
				s = strconv.Itoa(p.PublicPort) + "→" + s
			}
			if !seen[s] {
				seen[s] = true
				o.Ports = append(o.Ports, s)
			}
		}
		if c.State == "running" {
			if dir := cgroupDir(c.ID); dir != "" {
				usage, mem, lim := readCgroupStats(dir)
				o.Mem, o.MemLimit = mem, lim
				if p, ok := cstats.prev[c.ID]; ok && usage >= p.usage {
					if el := now.Sub(p.at).Nanoseconds(); el > 0 {
						o.CPU = round1(float64(usage-p.usage) / float64(el) * 100)
					}
				}
				cstats.prev[c.ID] = cgStat{usage: usage, at: now}
				live[c.ID] = true
			}
		}
		out[i] = o
		wg.Add(1)
		go func(i int, id string) { defer wg.Done(); out[i].Policy = restartPolicy(id) }(i, c.ID)
	}
	for id := range cstats.prev {
		if !live[id] {
			delete(cstats.prev, id)
			delete(cstats.paths, id)
		}
	}
	cstats.Unlock()
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		return out[i].Name < out[j].Name
	})
	_, cliErr := exec.LookPath("docker")
	writeJSON(w, r, map[string]any{"available": true, "containers": out, "cli": cliErr == nil})
}

var reCtrRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

func handleDockerInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !reCtrRef.MatchString(id) {
		httpErr(w, 400, "bad container id")
		return
	}
	var raw map[string]any
	if err := dockerJSON(r.Context(), "GET", "/containers/"+id+"/json", nil, &raw); err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	writeJSON(w, r, raw)
}

func handleDockerAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	if !reCtrRef.MatchString(id) {
		httpErr(w, 400, "bad container id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var err error
	switch action {
	case "start", "restart", "kill", "pause", "unpause":
		err = dockerJSON(ctx, "POST", "/containers/"+id+"/"+action, nil, nil)
	case "stop":
		err = dockerJSON(ctx, "POST", "/containers/"+id+"/stop?t=15", nil, nil)
	case "remove":
		err = dockerJSON(ctx, "DELETE", "/containers/"+id+"?force=1", nil, nil)
	case "policy":
		var req struct{ Policy string }
		readJSON(r, &req)
		switch req.Policy {
		case "no", "always", "unless-stopped", "on-failure":
		default:
			httpErr(w, 400, "bad restart policy")
			return
		}
		err = dockerJSON(ctx, "POST", "/containers/"+id+"/update", map[string]any{"RestartPolicy": map[string]any{"Name": req.Policy}}, nil)
		policies.Lock()
		policies.m = map[string]string{}
		policies.Unlock()
	default:
		httpErr(w, 400, "unknown action")
		return
	}
	invalidateContainers()
	if err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	ok(w, r)
}

// handleDockerLogs streams container logs as SSE, demultiplexing docker's stdout/stderr frame format.
func handleDockerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !reCtrRef.MatchString(id) {
		httpErr(w, 400, "bad container id")
		return
	}
	tail := r.URL.Query().Get("tail")
	if n, err := strconv.Atoi(tail); err != nil || n <= 0 || n > 5000 {
		tail = "300"
	}
	var ins struct{ Config struct{ Tty bool } }
	if err := dockerJSON(r.Context(), "GET", "/containers/"+id+"/json", nil, &ins); err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	resp, err := dockerReq(r.Context(), "GET", "/containers/"+id+"/logs?stdout=1&stderr=1&timestamps=1&follow=1&tail="+tail, nil)
	if err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	defer resp.Body.Close()
	ch := make(chan any, 256)
	go func() {
		defer close(ch)
		emit := func(stream int, data []byte) bool {
			for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
				ts, msg, _ := strings.Cut(l, " ")
				select {
				case ch <- []any{ts, stream, trimLine(msg)}:
				case <-r.Context().Done():
					return false
				}
			}
			return true
		}
		if ins.Config.Tty {
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 64<<10), 1<<20)
			for sc.Scan() {
				if !emit(1, sc.Bytes()) {
					return
				}
			}
			return
		}
		hdr := make([]byte, 8)
		br := bufio.NewReaderSize(resp.Body, 32<<10)
		var partial [3][]byte
		for {
			if _, err := io.ReadFull(br, hdr); err != nil {
				return
			}
			st := int(hdr[0])
			if st > 2 {
				st = 1
			}
			n := binary.BigEndian.Uint32(hdr[4:])
			if n > 8<<20 {
				return
			}
			buf := make([]byte, n)
			if _, err := io.ReadFull(br, buf); err != nil {
				return
			}
			// frames may split lines; hold back an unterminated tail
			buf = append(partial[st], buf...)
			cut := bytes.LastIndexByte(buf, '\n')
			if cut < 0 {
				partial[st] = buf
				continue
			}
			partial[st] = append([]byte(nil), buf[cut+1:]...)
			if !emit(st, buf[:cut]) {
				return
			}
		}
	}()
	lineStream(w, r, ch)
}

func handleDockerImages(w http.ResponseWriter, r *http.Request) {
	var imgs []struct {
		ID       string `json:"Id"`
		RepoTags []string
		Size     int64
		Created  int64
	}
	if err := dockerJSON(r.Context(), "GET", "/images/json", nil, &imgs); err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	list, _ := containers(time.Second)
	used := map[string]int{}
	for _, c := range list {
		used[c.ImageID]++
	}
	type img struct {
		ID      string   `json:"id"`
		Tags    []string `json:"tags"`
		Size    int64    `json:"size"`
		Created int64    `json:"created"`
		Used    int      `json:"used"`
	}
	out := make([]img, 0, len(imgs))
	for _, i := range imgs {
		tags := i.RepoTags
		if len(tags) == 1 && tags[0] == "<none>:<none>" {
			tags = nil
		}
		out = append(out, img{ID: i.ID, Tags: tags, Size: i.Size, Created: i.Created, Used: used[i.ID]})
	}
	var vols struct {
		Volumes []struct {
			Name, Driver, Mountpoint, CreatedAt string
		}
	}
	dockerJSON(r.Context(), "GET", "/volumes", nil, &vols)
	var nets []struct {
		Name, Driver, Scope string
		ID                  string `json:"Id"`
		IPAM                struct{ Config []struct{ Subnet string } }
	}
	dockerJSON(r.Context(), "GET", "/networks", nil, &nets)
	writeJSON(w, r, map[string]any{"images": out, "volumes": vols.Volumes, "networks": nets})
}

func handleDockerImageRemove(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string }
	if readJSON(r, &req) != nil || req.ID == "" {
		httpErr(w, 400, "missing image")
		return
	}
	if err := dockerJSON(r.Context(), "DELETE", "/images/"+url.PathEscape(req.ID), nil, nil); err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	ok(w, r)
}

func handleDockerPrune(w http.ResponseWriter, r *http.Request) {
	var req struct{ What string }
	readJSON(r, &req)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	p, found := map[string]string{
		"containers": "/containers/prune",
		"images":     "/images/prune?filters=" + url.QueryEscape(`{"dangling":["false"]}`),
		"dangling":   "/images/prune",
		"volumes":    "/volumes/prune",
		"networks":   "/networks/prune",
		"build":      "/build/prune",
	}[req.What]
	if !found {
		httpErr(w, 400, "unknown prune target")
		return
	}
	var res struct{ SpaceReclaimed uint64 }
	if err := dockerJSON(ctx, "POST", p, nil, &res); err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	invalidateContainers()
	writeJSON(w, r, map[string]any{"reclaimed": res.SpaceReclaimed})
}

// handleStack runs docker compose against a project's original files, streaming output.
func handleStack(w http.ResponseWriter, r *http.Request) {
	var req struct{ Project, Action string }
	if readJSON(r, &req) != nil || req.Project == "" {
		httpErr(w, 400, "missing project")
		return
	}
	list, err := containers(0)
	if err != nil {
		httpErr(w, 502, err.Error())
		return
	}
	var dir, files string
	for _, c := range list {
		if c.Labels["com.docker.compose.project"] == req.Project {
			dir = c.Labels["com.docker.compose.project.working_dir"]
			files = c.Labels["com.docker.compose.project.config_files"]
			break
		}
	}
	if dir == "" {
		httpErr(w, 404, "compose project not found")
		return
	}
	base := []string{"compose", "-p", req.Project}
	for _, f := range strings.Split(files, ",") {
		if f = strings.TrimSpace(f); f != "" {
			base = append(base, "-f", f)
		}
	}
	steps := map[string][][]string{
		"up":      {{"up", "-d", "--remove-orphans"}},
		"update":  {{"pull"}, {"up", "-d", "--remove-orphans"}},
		"restart": {{"restart"}},
		"stop":    {{"stop"}},
		"start":   {{"start"}},
		"down":    {{"down"}},
	}[req.Action]
	if steps == nil {
		httpErr(w, 400, "unknown stack action")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)
	fw := flushWriter{w, fl}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	for _, s := range steps {
		args := append(append([]string{}, base...), s...)
		fmt.Fprintf(fw, "$ docker %s\n", strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = fw, fw
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(fw, "\n✗ %v\n", err)
			return
		}
	}
	invalidateContainers()
	fmt.Fprint(fw, "\n✓ done\n")
}

type flushWriter struct {
	w  io.Writer
	fl http.Flusher
}

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if f.fl != nil {
		f.fl.Flush()
	}
	return n, err
}

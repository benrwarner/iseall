package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------- network ----------

type ifaceOut struct {
	Name    string   `json:"name"`
	MAC     string   `json:"mac"`
	MTU     int      `json:"mtu"`
	State   string   `json:"state"`
	Speed   int      `json:"speed"`
	Addrs   []string `json:"addrs"`
	Virtual bool     `json:"virtual"`
	RxTotal uint64   `json:"rxTotal"`
	TxTotal uint64   `json:"txTotal"`
	Errors  uint64   `json:"errors"`
	Drops   uint64   `json:"drops"`
	Wifi    *wifi    `json:"wifi,omitempty"`
}

type wifi struct {
	SSID    string  `json:"ssid"`
	Quality float64 `json:"quality"`
	Level   float64 `json:"level"`
}

func handleNetwork(w http.ResponseWriter, r *http.Request) {
	ifs, _ := net.Interfaces()
	counters := map[string][]uint64{}
	if b, err := os.ReadFile("/proc/net/dev"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			name, rest, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			var v []uint64
			for _, f := range strings.Fields(rest) {
				n, _ := strconv.ParseUint(f, 10, 64)
				v = append(v, n)
			}
			counters[strings.TrimSpace(name)] = v
		}
	}
	wl := map[string]*wifi{}
	if b, err := os.ReadFile("/proc/net/wireless"); err == nil {
		lines := strings.Split(string(b), "\n")
		if len(lines) > 2 {
			for _, l := range lines[2:] {
				name, rest, ok := strings.Cut(l, ":")
				f := strings.Fields(rest)
				if !ok || len(f) < 3 {
					continue
				}
				q, _ := strconv.ParseFloat(strings.TrimSuffix(f[1], "."), 64)
				lv, _ := strconv.ParseFloat(strings.TrimSuffix(f[2], "."), 64)
				wl[strings.TrimSpace(name)] = &wifi{Quality: round1(q * 100 / 70), Level: lv}
			}
		}
	}
	var out []ifaceOut
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 || strings.HasPrefix(i.Name, "veth") {
			continue
		}
		o := ifaceOut{Name: i.Name, MAC: i.HardwareAddr.String(), MTU: i.MTU, Virtual: isVirtualIface(i.Name),
			State: readTrim("/sys/class/net/" + i.Name + "/operstate")}
		o.Speed, _ = strconv.Atoi(readTrim("/sys/class/net/" + i.Name + "/speed"))
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			o.Addrs = append(o.Addrs, a.String())
		}
		if c := counters[i.Name]; len(c) >= 12 {
			o.RxTotal, o.TxTotal = c[0], c[8]
			o.Errors, o.Drops = c[2]+c[10], c[3]+c[11]
		}
		if wf := wl[i.Name]; wf != nil {
			if p, err := exec.LookPath("iwgetid"); err == nil {
				ssid, _ := exec.Command(p, i.Name, "-r").Output()
				wf.SSID = strings.TrimSpace(string(ssid))
			}
			o.Wifi = wf
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(a, b int) bool { return !out[a].Virtual && out[b].Virtual })

	var gw, gwIf string
	if b, err := os.ReadFile("/proc/net/route"); err == nil {
		for _, l := range strings.Split(string(b), "\n")[1:] {
			f := strings.Fields(l)
			if len(f) > 2 && f[1] == "00000000" {
				if raw, err := hex.DecodeString(f[2]); err == nil && len(raw) == 4 {
					gw = net.IPv4(raw[3], raw[2], raw[1], raw[0]).String()
					gwIf = f[0]
				}
				break
			}
		}
	}
	var dns []string
	if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "nameserver "); ok {
				dns = append(dns, strings.TrimSpace(v))
			}
		}
	}
	hn, _ := os.Hostname()
	writeJSON(w, r, map[string]any{"ifaces": out, "gateway": gw, "gatewayIf": gwIf, "dns": dns, "hostname": hn})
}

// ---------- storage ----------

func handleStorage(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, map[string]any{"mounts": readMounts(), "devices": readBlockDevs()})
}

// ---------- security ----------

var (
	reFailed   = regexp.MustCompile(`Failed \S+ for (?:invalid user )?(\S+) from (\S+)`)
	reInvalid  = regexp.MustCompile(`Invalid user (\S*) from (\S+)`)
	reAccepted = regexp.MustCompile(`Accepted (\S+) for (\S+) from (\S+)`)
)

type attempt struct {
	TS   int64  `json:"ts"`
	User string `json:"user"`
	IP   string `json:"ip"`
	OK   bool   `json:"ok"`
	How  string `json:"how,omitempty"`
}

type ipCount struct {
	IP string `json:"ip"`
	N  int    `json:"n"`
}

func handleSecurity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	res := map[string]any{}

	type session struct {
		User  string `json:"user"`
		TTY   string `json:"tty"`
		Since string `json:"since"`
		From  string `json:"from"`
	}
	sessions := []session{}
	if out, err := exec.CommandContext(ctx, "who").Output(); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			f := strings.Fields(l)
			if len(f) < 4 {
				continue
			}
			s := session{User: f[0], TTY: f[1], Since: strings.Join(f[2:4], " ")}
			if last := f[len(f)-1]; strings.HasPrefix(last, "(") {
				s.From = strings.Trim(last, "()")
			}
			sessions = append(sessions, s)
		}
	}
	res["sessions"] = sessions

	// sshd activity, last 24h
	cmd := exec.CommandContext(ctx, "journalctl", "--since", "-24h", "--no-pager", "-o", "short-unix", "_COMM=sshd", "_COMM=sshd-session")
	fails, accepts := []attempt{}, []attempt{}
	byIP := map[string]int{}
	if stdout, err := cmd.StdoutPipe(); err == nil && cmd.Start() == nil {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for i := 0; sc.Scan() && i < 100000; i++ {
			l := sc.Text()
			tsS, _, _ := strings.Cut(l, " ")
			tsF, _ := strconv.ParseFloat(tsS, 64)
			ts := int64(tsF)
			if m := reAccepted.FindStringSubmatch(l); m != nil {
				accepts = append(accepts, attempt{TS: ts, User: m[2], IP: m[3], OK: true, How: m[1]})
			} else if m := reFailed.FindStringSubmatch(l); m != nil {
				fails = append(fails, attempt{TS: ts, User: m[1], IP: m[2]})
				byIP[m[2]]++
			} else if m := reInvalid.FindStringSubmatch(l); m != nil {
				fails = append(fails, attempt{TS: ts, User: m[1], IP: m[2]})
				byIP[m[2]]++
			}
		}
		cmd.Wait()
	}
	top := []ipCount{}
	total := 0
	for ip, n := range byIP {
		top = append(top, ipCount{ip, n})
		total += n
	}
	sort.Slice(top, func(i, j int) bool { return top[i].N > top[j].N })
	if len(top) > 10 {
		top = top[:10]
	}
	if len(fails) > 50 {
		fails = fails[len(fails)-50:]
	}
	if len(accepts) > 20 {
		accepts = accepts[len(accepts)-20:]
	}
	res["failIPs"] = len(byIP)
	res["failTotal"] = total
	res["fails"] = fails
	res["accepts"] = accepts
	res["topIPs"] = top

	fw := "not detected"
	if p, err := exec.LookPath("ufw"); err == nil {
		if out, err := exec.CommandContext(ctx, p, "status").Output(); err == nil {
			fw = "ufw " + strings.TrimPrefix(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0], "Status: ")
		}
	} else if p, err := exec.LookPath("nft"); err == nil {
		if out, err := exec.CommandContext(ctx, p, "list", "ruleset").Output(); err == nil {
			if n := strings.Count(string(out), "\n"); n > 0 {
				fw = "nftables (" + strconv.Itoa(n) + " lines)"
			} else {
				fw = "nftables (empty)"
			}
		}
	}
	res["firewall"] = fw

	if b, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
		cv := map[string]string{}
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Fields(l)
			if len(f) >= 2 && !strings.HasPrefix(f[0], "#") {
				if _, seen := cv[strings.ToLower(f[0])]; !seen {
					cv[strings.ToLower(f[0])] = f[1]
				}
			}
		}
		res["sshd"] = map[string]string{"PermitRootLogin": cv["permitrootlogin"], "PasswordAuthentication": cv["passwordauthentication"], "Port": cv["port"]}
	}
	writeJSON(w, r, res)
}

func handleUpdates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := exec.LookPath("apt"); err != nil {
		writeJSON(w, r, map[string]any{"supported": false})
		return
	}
	out, _ := exec.CommandContext(ctx, "apt", "list", "--upgradable").Output()
	pk := []map[string]any{}
	for _, l := range strings.Split(string(out), "\n") {
		name, rest, ok := strings.Cut(l, "/")
		if !ok || strings.HasPrefix(l, "Listing") {
			continue
		}
		p := map[string]any{"name": name, "security": strings.Contains(rest, "-security")}
		if f := strings.Fields(rest); len(f) > 1 {
			p["version"] = f[1]
		}
		if i := strings.Index(l, "upgradable from: "); i >= 0 {
			p["from"] = strings.TrimSuffix(l[i+17:], "]")
		}
		pk = append(pk, p)
	}
	var listsAge int64
	if st, err := os.Stat("/var/lib/apt/lists"); err == nil {
		listsAge = int64(time.Since(st.ModTime()).Seconds())
	}
	_, rb := os.Stat("/var/run/reboot-required")
	writeJSON(w, r, map[string]any{"supported": true, "packages": pk, "listsAge": listsAge, "rebootRequired": rb == nil})
}

func handlePower(w http.ResponseWriter, r *http.Request) {
	var req struct{ Action string }
	readJSON(r, &req)
	verb := map[string]string{"reboot": "reboot", "shutdown": "poweroff"}[req.Action]
	if verb == "" {
		httpErr(w, 400, "unknown action")
		return
	}
	ok(w, r)
	go func() {
		time.Sleep(700 * time.Millisecond)
		exec.Command("systemctl", verb).Run()
	}()
}

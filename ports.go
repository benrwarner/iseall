package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type sockEnt struct {
	proto string
	lip   net.IP
	lport int
	rip   net.IP
	rport int
	state string
	inode string
}

var tcpStates = map[string]string{"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1", "05": "FIN_WAIT2",
	"06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING"}

func parseHexAddr(s string) (net.IP, int) {
	ipHex, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return nil, 0
	}
	port, _ := strconv.ParseUint(portHex, 16, 16)
	b, err := hex.DecodeString(ipHex)
	if err != nil {
		return nil, 0
	}
	// the kernel prints each 32-bit word in host (little-endian) order
	for j := 0; j+4 <= len(b); j += 4 {
		b[j], b[j+1], b[j+2], b[j+3] = b[j+3], b[j+2], b[j+1], b[j]
	}
	ip := net.IP(b)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip, int(port)
}

func readSockets() []sockEnt {
	var out []sockEnt
	for _, proto := range []string{"tcp", "tcp6", "udp", "udp6"} {
		b, err := os.ReadFile("/proc/net/" + proto)
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		for _, l := range lines[1:] {
			f := strings.Fields(l)
			if len(f) < 10 {
				continue
			}
			lip, lp := parseHexAddr(f[1])
			rip, rp := parseHexAddr(f[2])
			if lip == nil {
				continue
			}
			out = append(out, sockEnt{proto: strings.TrimSuffix(proto, "6"), lip: lip, lport: lp, rip: rip, rport: rp, state: f[3], inode: f[9]})
		}
	}
	return out
}

// inodeOwners walks /proc/*/fd once and maps socket inodes to pids.
func inodeOwners(want map[string]bool) map[string]int {
	res := make(map[string]int, len(want))
	ents, _ := os.ReadDir("/proc")
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := "/proc/" + e.Name() + "/fd/"
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			l, err := os.Readlink(fdDir + fd.Name())
			if err != nil || !strings.HasPrefix(l, "socket:[") {
				continue
			}
			ino := l[8 : len(l)-1]
			if want[ino] {
				if _, seen := res[ino]; !seen {
					res[ino] = pid
				}
			}
		}
	}
	return res
}

var wellKnown = map[int]string{20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns", 67: "dhcp", 68: "dhcp", 69: "tftp",
	80: "http", 110: "pop3", 111: "rpcbind", 123: "ntp", 137: "netbios", 138: "netbios", 139: "smb", 143: "imap", 161: "snmp",
	389: "ldap", 443: "https", 445: "smb", 465: "smtps", 514: "syslog", 587: "submission", 631: "ipp", 853: "dns-tls", 993: "imaps",
	995: "pop3s", 1194: "openvpn", 1883: "mqtt", 1900: "ssdp", 2049: "nfs", 2375: "docker", 2376: "docker-tls", 3000: "grafana",
	3306: "mysql", 3389: "rdp", 4533: "navidrome", 5000: "upnp/registry", 5353: "mdns", 5432: "postgres", 5900: "vnc", 6379: "redis",
	6443: "kubernetes", 8006: "proxmox", 8080: "http-alt", 8086: "influxdb", 8096: "jellyfin", 8123: "home-assistant",
	8200: "vault", 8443: "https-alt", 8765: "iseall", 8883: "mqtts", 9000: "portainer", 9090: "prometheus/cockpit", 9100: "node-exporter",
	9443: "portainer", 11211: "memcached", 19999: "netdata", 27017: "mongodb", 32400: "plex", 41641: "tailscale", 51820: "wireguard"}

var nonHTTP = map[int]bool{22: true, 23: true, 25: true, 53: true, 110: true, 111: true, 139: true, 143: true, 445: true, 465: true,
	587: true, 993: true, 995: true, 1883: true, 2049: true, 3306: true, 3389: true, 5432: true, 5900: true, 6379: true, 8883: true,
	11211: true, 27017: true}

type webInfo struct {
	Scheme string `json:"scheme"`
	Title  string `json:"title,omitempty"`
	Status int    `json:"status"`
}

type listener struct {
	Proto string   `json:"proto"`
	Port  int      `json:"port"`
	Addrs []string `json:"addrs"`
	PID   int      `json:"pid"`
	Proc  string   `json:"proc"`
	Ctr   string   `json:"ctr,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Svc   string   `json:"svc,omitempty"`
	Scope string   `json:"scope"` // public | lan | local
	Conns int      `json:"conns"`
	Web   *webInfo `json:"web,omitempty"`
}

type conn struct {
	Proto  string `json:"proto"`
	Local  string `json:"local"`
	LPort  int    `json:"lport"`
	Remote string `json:"remote"`
	RPort  int    `json:"rport"`
	State  string `json:"state"`
	PID    int    `json:"pid"`
	Proc   string `json:"proc"`
	In     bool   `json:"in"`
}

func scopeOf(ip net.IP) string {
	switch {
	case ip.IsUnspecified():
		return "public"
	case ip.IsLoopback():
		return "local"
	default:
		return "lan"
	}
}

func procName(pid int) string { return readTrim("/proc/" + strconv.Itoa(pid) + "/comm") }

func handlePorts(w http.ResponseWriter, r *http.Request) {
	socks := readSockets()
	want := map[string]bool{}
	listenPorts := map[string]bool{}
	for _, s := range socks {
		if s.inode != "0" {
			want[s.inode] = true
		}
		if s.state == "0A" {
			listenPorts["tcp:"+strconv.Itoa(s.lport)] = true
		}
	}
	owners := inodeOwners(want)
	names := ctrNames()
	pubs := dockerPublished()

	type key struct {
		proto string
		port  int
		pid   int
	}
	lm := map[key]*listener{}
	var conns []conn
	states := map[string]int{}
	estab := map[int]int{}
	for _, s := range socks {
		pid := owners[s.inode]
		if s.proto == "tcp" {
			states[tcpStates[s.state]]++
		}
		isListen := (s.proto == "tcp" && s.state == "0A") || (s.proto == "udp" && s.state == "07" && s.rport == 0)
		if !isListen {
			if s.proto == "tcp" && !s.rip.IsUnspecified() {
				in := listenPorts["tcp:"+strconv.Itoa(s.lport)]
				if s.state == "01" && in {
					estab[s.lport]++
				}
				if len(conns) < 400 && !(s.rip.IsLoopback() && s.lip.IsLoopback()) {
					conns = append(conns, conn{Proto: s.proto, Local: s.lip.String(), LPort: s.lport, Remote: s.rip.String(), RPort: s.rport,
						State: tcpStates[s.state], PID: pid, In: in})
				}
			}
			continue
		}
		k := key{s.proto, s.lport, pid}
		l := lm[k]
		if l == nil {
			l = &listener{Proto: s.proto, Port: s.lport, PID: pid, Svc: wellKnown[s.lport], Scope: "local"}
			if pid > 0 {
				l.Proc = procName(pid)
				id, unit := cgroupTags(pid)
				l.Unit = unit
				if id != "" {
					l.Ctr = names[id]
				}
			}
			lm[k] = l
		}
		if a := s.lip.String(); !containsStr(l.Addrs, a) {
			l.Addrs = append(l.Addrs, a)
		}
		// widest exposure wins
		if sc := scopeOf(s.lip); sc == "public" || (sc == "lan" && l.Scope == "local") {
			l.Scope = sc
		}
	}
	out := make([]listener, 0, len(lm))
	seenPort := map[string]bool{}
	for _, l := range lm {
		pk := l.Proto + ":" + strconv.Itoa(l.Port)
		if l.Proto == "tcp" {
			l.Conns = estab[l.Port]
		}
		if c, ok := pubs[pk]; ok && l.Ctr == "" {
			l.Ctr = c
		}
		seenPort[pk] = true
		out = append(out, *l)
	}
	// ports published by Docker via iptables only (userland-proxy disabled) have no listening socket
	for k, c := range pubs {
		if seenPort[k] {
			continue
		}
		p, ps, _ := strings.Cut(k, ":")
		n, _ := strconv.Atoi(ps)
		out = append(out, listener{Proto: p, Port: n, Addrs: []string{"0.0.0.0"}, Proc: "docker (nat)", Ctr: c, Scope: "public", Svc: wellKnown[n]})
	}
	probeWeb(out)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Proto < out[j].Proto
	})
	pn := map[int]string{}
	for i := range conns {
		if p := conns[i].PID; p > 0 {
			if _, ok := pn[p]; !ok {
				pn[p] = procName(p)
			}
			conns[i].Proc = pn[p]
		}
	}
	writeJSON(w, r, map[string]any{"listen": out, "conns": conns, "states": states})
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ---------- web UI discovery: probe local TCP listeners for an HTTP(S) page title ----------

var webCache = struct {
	sync.Mutex
	m   map[int]*webInfo
	at  map[int]time.Time
	inF map[int]bool
}{m: map[int]*webInfo{}, at: map[int]time.Time{}, inF: map[int]bool{}}

var reTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

var probeClient = &http.Client{
	Timeout: 1500 * time.Millisecond,
	Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // local self-signed UIs are the norm
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: 600 * time.Millisecond}).DialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !isLocalHost(req.URL.Hostname()) {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func isLocalHost(h string) bool {
	ip := net.ParseIP(h)
	return h == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate()))
}

func probeWeb(ls []listener) {
	webCache.Lock()
	defer webCache.Unlock()
	for i := range ls {
		l := &ls[i]
		if l.Proto != "tcp" || nonHTTP[l.Port] {
			continue
		}
		if wi, ok := webCache.m[l.Port]; ok {
			l.Web = wi
		}
		if time.Since(webCache.at[l.Port]) < 2*time.Minute || webCache.inF[l.Port] {
			continue
		}
		host := "127.0.0.1"
		if len(l.Addrs) > 0 {
			ip := net.ParseIP(l.Addrs[0])
			switch {
			case ip == nil:
			case ip.To4() == nil && ip.IsUnspecified():
				host = "[::1]"
			case !ip.IsUnspecified() && !ip.IsLoopback():
				host = l.Addrs[0]
				if ip.To4() == nil {
					host = "[" + host + "]"
				}
			}
		}
		webCache.inF[l.Port] = true
		go func(port int, host string) {
			wi := probeOne(host, port)
			webCache.Lock()
			webCache.at[port] = time.Now()
			delete(webCache.inF, port)
			if wi == nil {
				delete(webCache.m, port)
			} else {
				webCache.m[port] = wi
			}
			webCache.Unlock()
		}(l.Port, host)
	}
}

func probeOne(host string, port int) *webInfo {
	for _, scheme := range []string{"http", "https"} {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, "GET", scheme+"://"+host+":"+strconv.Itoa(port)+"/", nil)
		req.Header.Set("User-Agent", "iseall-probe")
		resp, err := probeClient.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		cancel()
		wi := &webInfo{Scheme: scheme, Status: resp.StatusCode}
		if m := reTitle.FindSubmatch(body); m != nil {
			t := strings.Join(strings.Fields(html.UnescapeString(string(m[1]))), " ")
			if len(t) > 80 {
				t = t[:80]
			}
			wi.Title = t
		}
		return wi
	}
	return nil
}

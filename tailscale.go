package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tailnet view: every device from `tailscale status --json`, plus a probe of each online peer
// for another iseall on the same port, so you can hop between dashboards.

var listenPort = 8765 // set from -addr in main

type tsNode struct {
	HostName       string
	DNSName        string
	OS             string
	TailscaleIPs   []string
	Online         bool
	ExitNode       bool
	ExitNodeOption bool
	LastSeen       time.Time
	Relay          string
	CurAddr        string
	RxBytes        int64
	TxBytes        int64
}

type tsStatus struct {
	BackendState   string
	Self           tsNode
	Peer           map[string]tsNode
	MagicDNSSuffix string
	CurrentTailnet *struct{ Name string }
}

type iseallPeer struct {
	URL     string `json:"url"`
	Version string `json:"version"`
	Setup   bool   `json:"setup"` // no password created yet
}

type tsDevice struct {
	Name     string      `json:"name"`
	DNS      string      `json:"dns"`
	IP       string      `json:"ip"`
	IPs      []string    `json:"ips"`
	OS       string      `json:"os"`
	Online   bool        `json:"online"`
	Self     bool        `json:"self"`
	LastSeen int64       `json:"lastSeen"`
	Direct   bool        `json:"direct"`
	Relay    string      `json:"relay"`
	Rx       int64       `json:"rx"`
	Tx       int64       `json:"tx"`
	ExitNode bool        `json:"exitNode"`
	ExitOpt  bool        `json:"exitOption"`
	Iseall   *iseallPeer `json:"iseall,omitempty"`
	Probed   bool        `json:"probed"`
}

var tsProbe = struct {
	sync.Mutex
	res map[string]*iseallPeer
	at  map[string]time.Time
}{res: map[string]*iseallPeer{}, at: map[string]time.Time{}}

var tsClient = &http.Client{
	Timeout: 2 * time.Second,
	Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func peerURL(ip string) string {
	return "http://" + net.JoinHostPort(ip, strconv.Itoa(listenPort)) + "/"
}

// probeIseall asks a peer's /api/auth (unauthenticated, tiny) and recognises iseall by its reply shape.
func probeIseall(ip string) *iseallPeer {
	resp, err := tsClient.Get(peerURL(ip) + "api/auth")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var a struct {
		Version *string `json:"version"`
		Setup   bool    `json:"setup"`
		Authed  *bool   `json:"authed"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&a) != nil || a.Version == nil || a.Authed == nil {
		return nil
	}
	return &iseallPeer{URL: peerURL(ip), Version: *a.Version, Setup: a.Setup}
}

func firstV4(ips []string) string {
	for _, ip := range ips {
		if strings.Contains(ip, ".") {
			return ip
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}

func handleTailscale(w http.ResponseWriter, r *http.Request) {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		writeJSON(w, r, map[string]any{"available": false, "reason": "Tailscale isn't installed on this machine."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "status", "--json").Output()
	var st tsStatus
	if jerr := json.Unmarshal(out, &st); jerr != nil {
		reason := "Couldn't read tailscale status."
		if err != nil {
			reason = "tailscale status failed: " + strings.TrimSpace(err.Error())
		}
		writeJSON(w, r, map[string]any{"available": false, "reason": reason})
		return
	}
	if st.BackendState != "Running" {
		writeJSON(w, r, map[string]any{"available": false, "state": st.BackendState,
			"reason": "Tailscale is " + strings.ToLower(st.BackendState) + " — run `sudo tailscale up` on this machine."})
		return
	}

	mk := func(n tsNode, self bool) tsDevice {
		d := tsDevice{Name: n.HostName, DNS: strings.TrimSuffix(n.DNSName, "."), IP: firstV4(n.TailscaleIPs), IPs: n.TailscaleIPs,
			OS: n.OS, Online: n.Online || self, Self: self, Direct: n.CurAddr != "", Relay: n.Relay, Rx: n.RxBytes, Tx: n.TxBytes,
			ExitNode: n.ExitNode, ExitOpt: n.ExitNodeOption}
		if n.LastSeen.Year() > 2000 {
			d.LastSeen = n.LastSeen.Unix()
		}
		if dn, _, ok := strings.Cut(d.DNS, "."); ok && dn != "" {
			d.Name = dn // the MagicDNS name is what people actually type
		}
		return d
	}
	devs := []tsDevice{mk(st.Self, true)}
	devs[0].Iseall = &iseallPeer{URL: peerURL(devs[0].IP), Version: version}
	devs[0].Probed = true
	for _, p := range st.Peer {
		devs = append(devs, mk(p, false))
	}

	// probe online peers in parallel; cached so a 15s poll doesn't hammer the tailnet
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	now := time.Now()
	for i := range devs {
		d := &devs[i]
		if d.Self || !d.Online || d.IP == "" || d.OS == "iOS" || d.OS == "android" {
			continue
		}
		tsProbe.Lock()
		res, at := tsProbe.res[d.IP], tsProbe.at[d.IP]
		tsProbe.Unlock()
		ttl := time.Minute
		if res != nil {
			ttl = 30 * time.Second
		}
		if !at.IsZero() && now.Sub(at) < ttl {
			d.Iseall, d.Probed = res, true
			continue
		}
		wg.Add(1)
		go func(d *tsDevice) {
			defer wg.Done()
			sem <- struct{}{}
			res := probeIseall(d.IP)
			<-sem
			tsProbe.Lock()
			tsProbe.res[d.IP], tsProbe.at[d.IP] = res, time.Now()
			tsProbe.Unlock()
			d.Iseall, d.Probed = res, true
		}(d)
	}
	wg.Wait()

	sort.SliceStable(devs, func(i, j int) bool {
		a, b := devs[i], devs[j]
		if a.Self != b.Self {
			return a.Self
		}
		if (a.Iseall != nil) != (b.Iseall != nil) {
			return a.Iseall != nil
		}
		if a.Online != b.Online {
			return a.Online
		}
		return a.Name < b.Name
	})
	tailnet := st.MagicDNSSuffix
	if st.CurrentTailnet != nil && st.CurrentTailnet.Name != "" {
		tailnet = st.CurrentTailnet.Name
	}
	writeJSON(w, r, map[string]any{"available": true, "tailnet": tailnet, "port": listenPort, "devices": devs})
}

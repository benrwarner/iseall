package main

import (
	"bytes"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------- static host info ----------

type SysInfo struct {
	Hostname string   `json:"hostname"`
	OS       string   `json:"os"`
	Kernel   string   `json:"kernel"`
	Arch     string   `json:"arch"`
	Model    string   `json:"model"`
	CPUModel string   `json:"cpuModel"`
	Cores    int      `json:"cores"`
	MemTotal uint64   `json:"memTotal"`
	BootTime int64    `json:"bootTime"`
	IsPi     bool     `json:"isPi"`
	Docker   bool     `json:"docker"`
	Version  string   `json:"version"`
	IPs      []string `json:"ips"`
}

func readSysInfo() SysInfo {
	si := SysInfo{Arch: runtime.GOARCH, Cores: runtime.NumCPU(), Version: version}
	si.Hostname, _ = os.Hostname()
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
				si.OS = strings.Trim(v, `"`)
			}
		}
	}
	var u syscall.Utsname
	if syscall.Uname(&u) == nil {
		si.Kernel = cstr(u.Release[:])
	}
	for _, p := range []string{"/proc/device-tree/model", "/sys/firmware/devicetree/base/model"} {
		if b, err := os.ReadFile(p); err == nil {
			si.Model = strings.TrimRight(string(b), "\x00\n ")
			break
		}
	}
	if si.Model == "" {
		si.Model = strings.TrimSpace(readTrim("/sys/class/dmi/id/sys_vendor") + " " + readTrim("/sys/class/dmi/id/product_name"))
	}
	si.IsPi = strings.Contains(si.Model, "Raspberry Pi")
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		part := ""
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if k == "model name" && si.CPUModel == "" {
				si.CPUModel = v
			}
			if k == "CPU part" && part == "" {
				part = v
			}
		}
		if si.CPUModel == "" {
			si.CPUModel = map[string]string{"0xd03": "Cortex-A53", "0xd07": "Cortex-A57", "0xd08": "Cortex-A72", "0xd0b": "Cortex-A76", "0xb76": "ARM1176"}[part]
		}
	}
	if mi := readMeminfo(); mi != nil {
		si.MemTotal = mi["MemTotal"]
	}
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "btime "); ok {
				si.BootTime, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	_, err := os.Stat(dockerSock)
	si.Docker = err == nil
	si.IPs = hostIPs()
	return si
}

func hostIPs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 || isVirtualIface(i.Name) {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}

func cstr[T int8 | uint8](b []T) string {
	var sb strings.Builder
	for _, c := range b {
		if c == 0 {
			break
		}
		sb.WriteByte(byte(c))
	}
	return sb.String()
}

func readTrim(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

func readMeminfo() map[string]uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
	}
	m := make(map[string]uint64, 16)
	for _, l := range bytes.Split(b, []byte{'\n'}) {
		k, v, ok := bytes.Cut(l, []byte{':'})
		if !ok {
			continue
		}
		f := bytes.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, _ := strconv.ParseUint(string(f[0]), 10, 64)
		m[string(k)] = n * 1024
	}
	return m
}

// ---------- live samples ----------

type IfRate struct {
	Name string  `json:"n"`
	Rx   float64 `json:"rx"`
	Tx   float64 `json:"tx"`
}

type Sample struct {
	T       int64      `json:"t"`
	CPU     float64    `json:"cpu"`
	IOWait  float64    `json:"iow"`
	Cores   []float64  `json:"cores"`
	MemT    uint64     `json:"memT"`
	MemU    uint64     `json:"memU"`
	MemC    uint64     `json:"memC"`
	SwapT   uint64     `json:"swapT"`
	SwapU   uint64     `json:"swapU"`
	Load    [3]float64 `json:"load"`
	Temp    float64    `json:"temp"`
	Freq    int        `json:"freq"`
	Rx      float64    `json:"rx"`
	Tx      float64    `json:"tx"`
	DR      float64    `json:"dr"`
	DW      float64    `json:"dw"`
	Up      float64    `json:"up"`
	Procs   int        `json:"procs"`
	Running int        `json:"run"`
	Thr     int64      `json:"thr"`
	Net     []IfRate   `json:"net"`
	Disk    []IfRate   `json:"disk"`
}

type cpuTimes struct{ busy, idle, iowait uint64 }

type sampler struct {
	prevT     time.Time
	prevCPU   []cpuTimes
	prevNet   map[string][2]uint64
	prevDisk  map[string][2]uint64
	tempPath  string
	freqPaths []string
	thrPath   string
	vcgencmd  string
	thrCache  int64
	thrAt     time.Time
	blockDevs map[string]bool
	blockAt   time.Time
}

func newSampler() *sampler {
	s := &sampler{prevNet: map[string][2]uint64{}, prevDisk: map[string][2]uint64{}, thrCache: -1}
	s.tempPath = findTempPath()
	s.freqPaths, _ = filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	if _, err := os.Stat("/sys/devices/platform/soc/soc:firmware/get_throttled"); err == nil {
		s.thrPath = "/sys/devices/platform/soc/soc:firmware/get_throttled"
	} else if p, err := exec.LookPath("vcgencmd"); err == nil {
		s.vcgencmd = p
	}
	return s
}

func findTempPath() string {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	best := ""
	for _, z := range zones {
		t := strings.ToLower(readTrim(z + "/type"))
		if strings.Contains(t, "cpu") || strings.Contains(t, "x86_pkg") || strings.Contains(t, "soc") {
			return z + "/temp"
		}
		if best == "" {
			best = z + "/temp"
		}
	}
	// hwmon fallback (k10temp/coretemp on PCs)
	hw, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hw {
		n := readTrim(h + "/name")
		if n == "k10temp" || n == "coretemp" || n == "cpu_thermal" {
			return h + "/temp1_input"
		}
	}
	return best
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func readCPU() []cpuTimes {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	out := make([]cpuTimes, 0, 9)
	for _, l := range bytes.Split(b, []byte{'\n'}) {
		if !bytes.HasPrefix(l, []byte("cpu")) {
			break
		}
		f := bytes.Fields(l)
		var v [10]uint64
		for i := 1; i < len(f) && i <= 10; i++ {
			v[i-1], _ = strconv.ParseUint(string(f[i]), 10, 64)
		}
		// user nice system idle iowait irq softirq steal (guest is already counted in user)
		total := v[0] + v[1] + v[2] + v[3] + v[4] + v[5] + v[6] + v[7]
		out = append(out, cpuTimes{busy: total - v[3] - v[4], idle: v[3], iowait: v[4]})
	}
	return out
}

func pct(dBusy, dTotal uint64) float64 {
	if dTotal == 0 {
		return 0
	}
	return round1(float64(dBusy) * 100 / float64(dTotal))
}

func (s *sampler) sample() Sample {
	now := time.Now()
	dt := now.Sub(s.prevT).Seconds()
	first := s.prevT.IsZero()
	s.prevT = now
	smp := Sample{T: now.UnixMilli()}

	if cpu := readCPU(); len(cpu) > 0 {
		if len(s.prevCPU) == len(cpu) {
			smp.Cores = make([]float64, len(cpu)-1)
			for i, c := range cpu {
				p := s.prevCPU[i]
				db := c.busy - p.busy
				dTot := db + (c.idle - p.idle) + (c.iowait - p.iowait)
				if i == 0 {
					smp.CPU = pct(db, dTot)
					smp.IOWait = pct(c.iowait-p.iowait, dTot)
				} else {
					smp.Cores[i-1] = pct(db, dTot)
				}
			}
		}
		s.prevCPU = cpu
	}

	if mi := readMeminfo(); mi != nil {
		smp.MemT = mi["MemTotal"]
		avail, okA := mi["MemAvailable"]
		if !okA {
			avail = mi["MemFree"] + mi["Buffers"] + mi["Cached"]
		}
		smp.MemU = smp.MemT - avail
		smp.MemC = mi["Buffers"] + mi["Cached"] + mi["SReclaimable"] - mi["Shmem"]
		smp.SwapT = mi["SwapTotal"]
		smp.SwapU = mi["SwapTotal"] - mi["SwapFree"]
	}

	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		if len(f) >= 4 {
			for i := 0; i < 3; i++ {
				smp.Load[i], _ = strconv.ParseFloat(f[i], 64)
			}
			r, t, _ := strings.Cut(f[3], "/")
			smp.Running, _ = strconv.Atoi(r)
			smp.Procs, _ = strconv.Atoi(t)
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			smp.Up, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	if s.tempPath != "" {
		if v, err := strconv.ParseFloat(readTrim(s.tempPath), 64); err == nil {
			smp.Temp = round1(v / 1000)
		}
	}
	if len(s.freqPaths) > 0 {
		var sum int
		for _, p := range s.freqPaths {
			v, _ := strconv.Atoi(readTrim(p))
			sum += v
		}
		smp.Freq = sum / len(s.freqPaths) / 1000
	}
	smp.Thr = s.throttled(now)

	// network
	if b, err := os.ReadFile("/proc/net/dev"); err == nil {
		cur := map[string][2]uint64{}
		lines := strings.Split(string(b), "\n")
		if len(lines) > 2 {
			lines = lines[2:]
		}
		for _, l := range lines {
			name, rest, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			f := strings.Fields(rest)
			if len(f) < 9 || name == "lo" {
				continue
			}
			rx, _ := strconv.ParseUint(f[0], 10, 64)
			tx, _ := strconv.ParseUint(f[8], 10, 64)
			cur[name] = [2]uint64{rx, tx}
			if p, ok := s.prevNet[name]; ok && !first && dt > 0 && rx >= p[0] && tx >= p[1] {
				r := IfRate{Name: name, Rx: math.Round(float64(rx-p[0]) / dt), Tx: math.Round(float64(tx-p[1]) / dt)}
				if !strings.HasPrefix(name, "veth") {
					smp.Net = append(smp.Net, r)
				}
				if !isVirtualIface(name) {
					smp.Rx += r.Rx
					smp.Tx += r.Tx
				}
			}
		}
		s.prevNet = cur
	}

	// disk I/O (whole devices only)
	if now.Sub(s.blockAt) > time.Minute {
		s.blockDevs = map[string]bool{}
		ents, _ := os.ReadDir("/sys/block")
		for _, e := range ents {
			n := e.Name()
			if !strings.HasPrefix(n, "loop") && !strings.HasPrefix(n, "ram") && !strings.HasPrefix(n, "zram") {
				s.blockDevs[n] = true
			}
		}
		s.blockAt = now
	}
	if b, err := os.ReadFile("/proc/diskstats"); err == nil {
		cur := map[string][2]uint64{}
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Fields(l)
			if len(f) < 10 || !s.blockDevs[f[2]] {
				continue
			}
			rd, _ := strconv.ParseUint(f[5], 10, 64)
			wr, _ := strconv.ParseUint(f[9], 10, 64)
			rd, wr = rd*512, wr*512
			cur[f[2]] = [2]uint64{rd, wr}
			if p, ok := s.prevDisk[f[2]]; ok && !first && dt > 0 && rd >= p[0] && wr >= p[1] {
				r := IfRate{Name: f[2], Rx: math.Round(float64(rd-p[0]) / dt), Tx: math.Round(float64(wr-p[1]) / dt)}
				smp.Disk = append(smp.Disk, r)
				smp.DR += r.Rx
				smp.DW += r.Tx
			}
		}
		s.prevDisk = cur
	}
	return smp
}

func (s *sampler) throttled(now time.Time) int64 {
	if s.thrPath != "" {
		v, err := strconv.ParseInt(readTrim(s.thrPath), 16, 64)
		if err != nil {
			return -1
		}
		return v
	}
	if s.vcgencmd == "" {
		return -1
	}
	if now.Sub(s.thrAt) > 10*time.Second {
		s.thrAt = now
		if out, err := exec.Command(s.vcgencmd, "get_throttled").Output(); err == nil {
			_, v, _ := strings.Cut(strings.TrimSpace(string(out)), "=")
			s.thrCache, _ = strconv.ParseInt(strings.TrimPrefix(v, "0x"), 16, 64)
		}
	}
	return s.thrCache
}

var virtCache sync.Map

// isVirtualIface reports interfaces with no backing device (docker0, br-*, veth*, wg0, tailscale0...).
func isVirtualIface(name string) bool {
	if v, ok := virtCache.Load(name); ok {
		return v.(bool)
	}
	_, err := os.Stat("/sys/class/net/" + name + "/device")
	virt := err != nil
	virtCache.Store(name, virt)
	return virt
}

// ---------- filesystems ----------

type Mount struct {
	Dev   string  `json:"dev"`
	Path  string  `json:"path"`
	FS    string  `json:"fs"`
	Total uint64  `json:"total"`
	Used  uint64  `json:"used"`
	Free  uint64  `json:"free"`
	Inode float64 `json:"inode"`
	RO    bool    `json:"ro"`
	Err   string  `json:"err,omitempty"`
}

var realFS = map[string]bool{"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "vfat": true, "exfat": true,
	"ntfs": true, "ntfs3": true, "fuseblk": true, "f2fs": true, "zfs": true, "nfs": true, "nfs4": true, "cifs": true, "smb3": true}

var statfsBusy sync.Map

func readMounts() []Mount {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Mount
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 4 || !realFS[f[2]] {
			continue
		}
		path := unescapeMount(f[1])
		if strings.HasPrefix(path, "/var/lib/docker") || strings.HasPrefix(path, "/snap") || strings.HasPrefix(path, "/run/") || strings.HasPrefix(path, "/boot/efi") {
			continue
		}
		if seen[f[0]] && f[2] != "zfs" {
			continue
		}
		seen[f[0]] = true
		m := Mount{Dev: f[0], Path: path, FS: f[2], RO: strings.HasPrefix(f[3], "ro,") || f[3] == "ro"}
		// statfs can block forever on a dead network mount; never let it stall the sampler.
		if _, busy := statfsBusy.Load(path); busy {
			m.Err = "not responding"
			out = append(out, m)
			continue
		}
		done := make(chan syscall.Statfs_t, 1)
		statfsBusy.Store(path, true)
		go func() {
			var st syscall.Statfs_t
			if syscall.Statfs(path, &st) == nil {
				done <- st
			}
			statfsBusy.Delete(path)
			close(done)
		}()
		select {
		case st, ok := <-done:
			if ok {
				bs := uint64(st.Bsize)
				m.Total = st.Blocks * bs
				m.Free = st.Bavail * bs
				m.Used = (st.Blocks - st.Bfree) * bs
				if st.Files > 0 {
					m.Inode = round1(float64(st.Files-st.Ffree) * 100 / float64(st.Files))
				}
			}
		case <-time.After(800 * time.Millisecond):
			m.Err = "not responding"
		}
		out = append(out, m)
	}
	return out
}

func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

type BlockDev struct {
	Name  string   `json:"name"`
	Size  uint64   `json:"size"`
	Model string   `json:"model"`
	Rota  bool     `json:"rota"`
	Rem   bool     `json:"rem"`
	Parts []string `json:"parts"`
}

func readBlockDevs() []BlockDev {
	ents, _ := os.ReadDir("/sys/block")
	var out []BlockDev
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "loop") || strings.HasPrefix(n, "ram") || strings.HasPrefix(n, "zram") {
			continue
		}
		base := "/sys/block/" + n
		sz, _ := strconv.ParseUint(readTrim(base+"/size"), 10, 64)
		if sz == 0 {
			continue
		}
		d := BlockDev{Name: n, Size: sz * 512, Model: readTrim(base + "/device/model"), Rota: readTrim(base+"/queue/rotational") == "1", Rem: readTrim(base+"/removable") == "1"}
		if d.Model == "" {
			d.Model = readTrim(base + "/device/name")
		}
		parts, _ := filepath.Glob(base + "/" + n + "*")
		for _, p := range parts {
			d.Parts = append(d.Parts, filepath.Base(p))
		}
		out = append(out, d)
	}
	return out
}

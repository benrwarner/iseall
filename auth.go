package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Thresholds struct {
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Temp float64 `json:"temp"`
	Disk float64 `json:"disk"`
}

type Config struct {
	PassHash string     `json:"pass_hash,omitempty"`
	Salt     string     `json:"salt,omitempty"`
	Secret   string     `json:"secret,omitempty"`
	Alerts   Thresholds `json:"alerts"`
	NoPulse  bool       `json:"no_pulse,omitempty"` // stop sharing the health summary with tailnet peers

	mu   sync.RWMutex
	path string
}

var cfg = &Config{}

const (
	pbkdfIter  = 120_000
	cookieName = "iseall_s"
	sessionTTL = 30 * 24 * time.Hour
)

func defaultConfigPath() string {
	if os.Geteuid() == 0 {
		return "/etc/iseall/config.json"
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "iseall", "config.json")
}

func loadConfig(p string) error {
	cfg.path = p
	cfg.Alerts = Thresholds{CPU: 90, Mem: 90, Temp: 75, Disk: 90}
	b, err := os.ReadFile(p)
	if err == nil {
		if err := json.Unmarshal(b, cfg); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if cfg.Secret == "" {
		cfg.Secret = randHex(32)
		return cfg.save()
	}
	return nil
}

func (c *Config) save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func (c *Config) hasPassword() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PassHash != ""
}

func hashPassword(pw string, salt []byte) string {
	k, _ := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, 32)
	return hex.EncodeToString(k)
}

// setPassword also rotates the session secret, logging out every other session.
func (c *Config) setPassword(pw string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	salt := make([]byte, 16)
	rand.Read(salt)
	c.Salt = hex.EncodeToString(salt)
	c.PassHash = hashPassword(pw, salt)
	c.Secret = randHex(32)
	return c.save()
}

func (c *Config) checkPassword(pw string) bool {
	c.mu.RLock()
	salt, _ := hex.DecodeString(c.Salt)
	want, _ := hex.DecodeString(c.PassHash)
	c.mu.RUnlock()
	got, _ := hex.DecodeString(hashPassword(pw, salt))
	return len(want) > 0 && hmac.Equal(got, want)
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func runPasswd() {
	fmt.Print("New dashboard password (min 8 chars): ")
	sc := bufio.NewScanner(os.Stdin)
	sc.Scan()
	pw := strings.TrimSpace(sc.Text())
	if len(pw) < 8 {
		log.Fatal("password too short")
	}
	if err := cfg.setPassword(pw); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Password updated. Restart iseall for running sessions to be invalidated.")
}

// ---------- stateless signed session cookie: "<expiry>.<hmac>" ----------

func sign(exp string) string {
	cfg.mu.RLock()
	m := hmac.New(sha256.New, []byte(cfg.Secret))
	cfg.mu.RUnlock()
	m.Write([]byte(exp))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func issueSession(w http.ResponseWriter, r *http.Request) {
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: exp + "." + sign(exp), Path: "/",
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil,
	})
}

func validSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	exp, sig, found := strings.Cut(c.Value, ".")
	if !found {
		return false
	}
	t, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > t {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(sign(exp)))
}

func requireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r) {
			httpErr(w, 401, "unauthorized")
			return
		}
		// CSRF: a custom header can't be set cross-origin without a CORS preflight we never grant.
		if r.Method != http.MethodGet && r.Header.Get("X-Iseall") != "1" {
			httpErr(w, 403, "missing request header")
			return
		}
		if r.Header.Get("Upgrade") != "" && !sameOrigin(r) {
			httpErr(w, 403, "bad origin")
			return
		}
		next(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

// ---------- brute-force protection ----------

var fails = struct {
	sync.Mutex
	m map[string]*failRec
}{m: map[string]*failRec{}}

type failRec struct {
	n     int
	until time.Time
}

func clientIP(r *http.Request) string {
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

func blocked(ip string) time.Duration {
	fails.Lock()
	defer fails.Unlock()
	if f := fails.m[ip]; f != nil && time.Now().Before(f.until) {
		return time.Until(f.until)
	}
	return 0
}

func recordFail(ip string) {
	fails.Lock()
	defer fails.Unlock()
	f := fails.m[ip]
	if f == nil {
		f = &failRec{}
		fails.m[ip] = f
	}
	f.n++
	if f.n >= 5 {
		f.until = time.Now().Add(time.Duration(f.n-4) * 30 * time.Second)
	}
	log.Printf("auth: failed login from %s (%d)", ip, f.n)
}

// ---------- handlers ----------

func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, map[string]any{"setup": !cfg.hasPassword(), "authed": validSession(r), "version": version})
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Iseall") != "1" {
		httpErr(w, 403, "missing request header")
		return
	}
	var req struct{ Password string }
	if err := readJSON(r, &req); err != nil || len(req.Password) < 8 {
		httpErr(w, 400, "password must be at least 8 characters")
		return
	}
	cfg.mu.RLock()
	done := cfg.PassHash != ""
	cfg.mu.RUnlock()
	if done {
		httpErr(w, 403, "already configured")
		return
	}
	if err := cfg.setPassword(req.Password); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	issueSession(w, r)
	ok(w, r)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if d := blocked(ip); d > 0 {
		httpErr(w, 429, fmt.Sprintf("too many attempts — try again in %ds", int(d.Seconds())+1))
		return
	}
	var req struct{ Password string }
	if readJSON(r, &req) != nil || !cfg.checkPassword(req.Password) {
		recordFail(ip)
		time.Sleep(400 * time.Millisecond)
		httpErr(w, 401, "wrong password")
		return
	}
	fails.Lock()
	delete(fails.m, ip)
	fails.Unlock()
	issueSession(w, r)
	ok(w, r)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	ok(w, r)
}

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct{ Old, New string }
	if readJSON(r, &req) != nil || len(req.New) < 8 {
		httpErr(w, 400, "new password must be at least 8 characters")
		return
	}
	if !cfg.checkPassword(req.Old) {
		httpErr(w, 401, "current password is wrong")
		return
	}
	if err := cfg.setPassword(req.New); err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	issueSession(w, r)
	ok(w, r)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var t Thresholds
		if readJSON(r, &t) != nil || t.CPU <= 0 || t.Mem <= 0 || t.Temp <= 0 || t.Disk <= 0 {
			httpErr(w, 400, "invalid thresholds")
			return
		}
		cfg.mu.Lock()
		cfg.Alerts = t
		err := cfg.save()
		cfg.mu.Unlock()
		if err != nil {
			httpErr(w, 500, err.Error())
			return
		}
	}
	cfg.mu.RLock()
	t, share := cfg.Alerts, !cfg.NoPulse
	cfg.mu.RUnlock()
	writeJSON(w, r, map[string]any{"alerts": t, "version": version, "pulse": share})
}

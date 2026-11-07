// iseall — a tiny, single-binary server dashboard for Raspberry Pi and other Linux boxes.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

var version = "1.0.0"

func main() {
	addr := flag.String("addr", ":8765", "listen address")
	cfgPath := flag.String("config", defaultConfigPath(), "config file path")
	certFile := flag.String("cert", "", "TLS certificate (optional)")
	keyFile := flag.String("key", "", "TLS key (optional)")
	setPass := flag.Bool("passwd", false, "set the dashboard password and exit")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if _, p, err := net.SplitHostPort(*addr); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			listenPort = n // peers are probed for iseall on the same port
		}
	}
	if *showVer {
		fmt.Println(version)
		return
	}
	// Small heap, frequent collection: we'd rather spend a few µs of CPU than hold RAM on a Pi.
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(48 << 20)

	if err := loadConfig(*cfgPath); err != nil {
		log.Fatalf("config: %v", err)
	}
	if *setPass {
		runPasswd()
		return
	}
	if os.Geteuid() != 0 {
		log.Printf("warning: not running as root — some processes, ports and actions will be hidden or fail")
	}

	h := newHub()
	go h.run()
	// flush long-term history on stop/reboot so rolling averages survive restarts
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		<-c
		h.trend.save()
		os.Exit(0)
	}()

	mux := http.NewServeMux()
	registerRoutes(mux, h)
	mux.Handle("GET /", staticHandler())

	srv := &http.Server{
		Addr:              *addr,
		Handler:           withHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("iseall %s listening on %s", version, *addr)
	if *certFile != "" && *keyFile != "" {
		log.Fatal(srv.ListenAndServeTLS(*certFile, *keyFile))
	}
	log.Fatal(srv.ListenAndServe())
}

func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// ---------- static files: embedded, pre-gzipped once at startup ----------

type staticFile struct {
	raw, gz []byte
	etag    string
	ctype   string
	immut   bool
}

func staticHandler() http.Handler {
	files := map[string]*staticFile{}
	sub, _ := fs.Sub(webFS, "web")
	fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, _ := fs.ReadFile(sub, p)
		sum := sha256.Sum256(raw)
		f := &staticFile{raw: raw, etag: `"` + hex.EncodeToString(sum[:8]) + `"`, immut: strings.HasPrefix(p, "vendor/")}
		f.ctype = mime.TypeByExtension(path.Ext(p))
		if f.ctype == "" {
			f.ctype = "application/octet-stream"
		}
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		zw.Write(raw)
		zw.Close()
		if b.Len() < len(raw) {
			f.gz = b.Bytes()
		}
		files["/"+p] = f
		return nil
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := files[r.URL.Path]
		if !ok {
			// SPA: every unknown non-API path gets the shell.
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			f = files["/index.html"]
		}
		h := w.Header()
		h.Set("Content-Type", f.ctype)
		h.Set("ETag", f.etag)
		if f.immut {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		h.Set("Vary", "Accept-Encoding")
		if r.Header.Get("If-None-Match") == f.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if f.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.Set("Content-Encoding", "gzip")
			w.Write(f.gz)
			return
		}
		w.Write(f.raw)
	})
}

// ---------- JSON helpers ----------

var gzPool = sync.Pool{New: func() any { w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed); return w }}

func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if len(b) > 1400 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzPool.Get().(*gzip.Writer)
		zw.Reset(w)
		zw.Write(b)
		zw.Close()
		gzPool.Put(zw)
		return
	}
	w.Write(b)
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func ok(w http.ResponseWriter, r *http.Request) { writeJSON(w, r, map[string]bool{"ok": true}) }

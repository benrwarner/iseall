package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// ---------- minimal RFC 6455 WebSocket server (stdlib only) ----------

type wsConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Sec-WebSocket-Key") == "" {
		return nil, errors.New("not a websocket request")
	}
	sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("hijack unsupported")
	}
	c, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
	if err := brw.Flush(); err != nil {
		c.Close()
		return nil, err
	}
	return &wsConn{c: c, br: brw.Reader}, nil
}

func (ws *wsConn) write(op byte, p []byte) error {
	ws.wmu.Lock()
	defer ws.wmu.Unlock()
	hdr := make([]byte, 2, 10+len(p))
	hdr[0] = 0x80 | op
	switch n := len(p); {
	case n < 126:
		hdr[1] = byte(n)
	case n < 65536:
		hdr[1] = 126
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr[1] = 127
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	_, err := ws.c.Write(append(hdr, p...))
	return err
}

// read returns the next complete data message, answering pings transparently.
func (ws *wsConn) read() (byte, []byte, error) {
	var msg []byte
	var msgOp byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(ws.br, h[:]); err != nil {
			return 0, nil, err
		}
		fin, op, masked := h[0]&0x80 != 0, h[0]&0x0f, h[1]&0x80 != 0
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(ws.br, b[:]); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(ws.br, b[:]); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > 1<<20 || !masked {
			return 0, nil, errors.New("bad frame")
		}
		var key [4]byte
		if _, err := io.ReadFull(ws.br, key[:]); err != nil {
			return 0, nil, err
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(ws.br, p); err != nil {
			return 0, nil, err
		}
		for i := range p {
			p[i] ^= key[i%4]
		}
		switch op {
		case 0x8:
			ws.write(0x8, nil)
			return 0, nil, io.EOF
		case 0x9:
			ws.write(0xA, p)
			continue
		case 0xA:
			continue
		case 0x0:
			msg = append(msg, p...)
		default:
			msgOp, msg = op, p
		}
		if fin {
			return msgOp, msg, nil
		}
	}
}

// ---------- pseudo-terminal without cgo ----------

func ioctl(fd, req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}

func startPTY(cmd *exec.Cmd) (*os.File, error) {
	fd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var unlock int32
	if err := ioctl(uintptr(fd), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	var n uint32
	if err := ioctl(uintptr(fd), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}
	defer slave.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	// non-blocking so the runtime poller owns it and Close() unblocks pending reads
	syscall.SetNonblock(fd, true)
	return os.NewFile(uintptr(fd), "/dev/ptmx"), nil
}

func resizePTY(f *os.File, cols, rows int) {
	if cols <= 0 || rows <= 0 || cols > 1000 || rows > 1000 {
		return
	}
	ws := [4]uint16{uint16(rows), uint16(cols), 0, 0}
	if rc, err := f.SyscallConn(); err == nil {
		rc.Control(func(fd uintptr) { ioctl(fd, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws))) })
	}
}

func loginShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	for _, s := range []string{"/bin/bash", "/bin/zsh", "/bin/sh"} {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	return "/bin/sh"
}

func handleTerm(w http.ResponseWriter, r *http.Request) {
	var cmd *exec.Cmd
	if id := r.URL.Query().Get("ctr"); id != "" {
		if !reCtrRef.MatchString(id) {
			httpErr(w, 400, "bad container id")
			return
		}
		cmd = exec.Command("docker", "exec", "-it", "-e", "TERM=xterm-256color", id, "sh", "-c", "command -v bash >/dev/null && exec bash || exec sh")
	} else {
		cmd = exec.Command(loginShell(), "-l")
		home, _ := os.UserHomeDir()
		cmd.Dir = home
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")

	ws, err := wsUpgrade(w, r)
	if err != nil {
		httpErr(w, 400, err.Error())
		return
	}
	defer ws.c.Close()
	pty, err := startPTY(cmd)
	if err != nil {
		ws.write(0x2, []byte(fmt.Sprintf("\r\n\x1b[31mfailed to start terminal: %v\x1b[0m\r\n", err)))
		ws.write(0x8, nil)
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	resizePTY(pty, cols, rows)

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
			pty.Close()
			ws.c.Close()
		})
	}
	defer func() { cleanup(); cmd.Wait() }()

	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := pty.Read(buf)
			if n > 0 && ws.write(0x2, buf[:n]) != nil {
				break
			}
			if err != nil {
				break
			}
		}
		ws.write(0x8, nil)
		cleanup()
	}()
	for {
		op, msg, err := ws.read()
		if err != nil {
			return
		}
		if op == 0x1 { // control message
			var c struct{ R [2]int }
			if json.Unmarshal(msg, &c) == nil {
				resizePTY(pty, c.R[0], c.R[1])
			}
			continue
		}
		if _, err := pty.Write(msg); err != nil {
			return
		}
	}
}

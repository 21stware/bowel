// Package owner tells the core on Android which app opened a connection,
// which a rule for an app needs: PROCESS-NAME with the app's package name.
//
// From Android 10 an app may not read the kernel's socket tables. Only the
// active VPN app may ask Android who owns a connection
// (ConnectivityManager.getConnectionOwnerUid), and the core is a process of
// its own, with no Java to call. So the core listens on a unix socket in the
// app's private folder, and the VPN service connects to it and answers one
// question per line on each connection it opened:
//
//	core:    tcp 198.18.0.1:40312 198.18.0.9:443
//	service: 10234 com.openai.chatgpt
//
// The uid is -1 when Android does not know the connection, and the package
// is empty when no package has the uid.
//
// It has no dependencies, so that mihomo's build can take it in: see
// mobile/android/build.sh, which also adds mihomo/owner_android.go to the
// core.
package owner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SocketEnv names the variable the launcher sets to the socket's path.
const SocketEnv = "BOWEL_OWNER_SOCKET"

// Timeout bounds a lookup. The connection waits for it, so a slow answer is
// given up and the connection matched as if no app had opened it.
const Timeout = 300 * time.Millisecond

// maxLines is how many of the service's connections are kept. Each carries
// one question at a time.
const maxLines = 8

var (
	ErrNoService = errors.New("the VPN service is not answering")
	ErrNotFound  = errors.New("no app owns the connection")
	ErrNetwork   = errors.New("only tcp and udp connections have owners")
	ErrTimeout   = errors.New("the VPN service took too long to answer")
)

// Server is the core's end of the socket.
type Server struct {
	l       net.Listener
	idle    chan *line // the service's connections not asked anything now
	open    atomic.Int32
	done    chan struct{}
	stop    sync.Once
	timeout time.Duration
}

// line is one of the service's connections.
type line struct {
	c net.Conn
	r *bufio.Reader
}

// Listen opens the socket at path, readable by this app's user only, and
// takes the service's connections as they come.
func Listen(path string) (*Server, error) {
	_ = os.Remove(path)
	l, err := listenPrivate(path)
	if err != nil {
		return nil, err
	}
	s := &Server{l: l, idle: make(chan *line, maxLines), done: make(chan struct{}), timeout: Timeout}
	go s.accept()
	return s, nil
}

func (s *Server) accept() {
	for {
		c, err := s.l.Accept()
		if err != nil {
			return // closed: the core is going away
		}
		if s.open.Add(1) > maxLines {
			s.open.Add(-1)
			_ = c.Close()
			continue
		}
		s.put(&line{c: c, r: bufio.NewReader(c)})
	}
}

// Close stops listening and closes the service's connections.
func (s *Server) Close() error {
	var err error
	s.stop.Do(func() {
		close(s.done)
		err = s.l.Close()
		for {
			select {
			case ln := <-s.idle:
				s.drop(ln)
			default:
				return
			}
		}
	})
	return err
}

// Lookup asks who owns the connection from src to dst over network, "tcp"
// or "udp": the uid, and the package it belongs to, which may be "". It
// fails at once while the service has no connection open, as on Android
// before 10, where it never connects.
func (s *Server) Lookup(network string, src, dst netip.AddrPort) (uint32, string, error) {
	if network != "tcp" && network != "udp" {
		return 0, "", ErrNetwork
	}
	q := fmt.Sprintf("%s %s %s\n", network, unmap(src), unmap(dst))
	deadline := time.Now().Add(s.timeout)
	for {
		if s.open.Load() == 0 {
			return 0, "", ErrNoService
		}
		ln, err := s.take(deadline)
		if err != nil {
			return 0, "", err
		}
		reply, err := ln.ask(q, deadline)
		if err != nil {
			// Its answer may still come, and would be taken for the next
			// question's: the line goes. The service opens another.
			s.drop(ln)
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return 0, "", ErrTimeout
			}
			continue // one the service closed: another may answer in time
		}
		s.put(ln)
		return parse(reply)
	}
}

// take waits for a line no other lookup is using.
func (s *Server) take(deadline time.Time) (*line, error) {
	select {
	case ln := <-s.idle:
		return ln, nil
	default:
	}
	t := time.NewTimer(time.Until(deadline))
	defer t.Stop()
	select {
	case ln := <-s.idle:
		return ln, nil
	case <-t.C:
		return nil, ErrTimeout
	case <-s.done:
		return nil, ErrNoService
	}
}

func (s *Server) put(ln *line) {
	select {
	case <-s.done:
		s.drop(ln)
		return
	default:
	}
	select {
	case s.idle <- ln:
	default: // never: there are no more lines than room
		s.drop(ln)
	}
}

func (s *Server) drop(ln *line) {
	_ = ln.c.Close()
	s.open.Add(-1)
}

func (ln *line) ask(q string, deadline time.Time) (string, error) {
	if err := ln.c.SetDeadline(deadline); err != nil {
		return "", err
	}
	if _, err := io.WriteString(ln.c, q); err != nil {
		return "", err
	}
	return ln.r.ReadString('\n')
}

// parse reads the service's answer: "uid package".
func parse(reply string) (uint32, string, error) {
	reply = strings.TrimRight(reply, "\r\n")
	id, pkg, _ := strings.Cut(reply, " ")
	uid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("the VPN service answered %q", reply)
	}
	if uid < 0 || uid > math.MaxUint32 {
		return 0, "", ErrNotFound
	}
	return uint32(uid), pkg, nil
}

// unmap writes an IPv4 address as Android knows it, not as IPv6.
func unmap(a netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(a.Addr().Unmap(), a.Port())
}

// AddrPort is a connection's end as the core's metadata has it: a, the
// address the TUN saw, when it is a TCP or a UDP one, else ip and port. A
// connection to a fake IP keeps that IP in a, as the app's socket does.
func AddrPort(a net.Addr, ip netip.Addr, port uint16) netip.AddrPort {
	switch a := a.(type) {
	case *net.TCPAddr:
		if a != nil {
			return a.AddrPort()
		}
	case *net.UDPAddr:
		if a != nil {
			return a.AddrPort()
		}
	}
	return netip.AddrPortFrom(ip, port)
}

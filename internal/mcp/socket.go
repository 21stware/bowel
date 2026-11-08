package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// ErrRunning is Listen's error when another instance answers on the path.
var ErrRunning = errors.New("mcp: another instance is listening")

// Listen opens the unix socket at path, for this user only. A socket left
// by a crash is replaced; one that answers is another instance's.
func Listen(path string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return nil, ErrRunning
	}
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Accept serves each connection to l until ctx is done, then closes l and
// the connections still open.
func (srv *Server) Accept(ctx context.Context, l net.Listener) {
	var (
		mu    sync.Mutex
		conns = map[net.Conn]bool{}
	)
	go func() {
		<-ctx.Done()
		l.Close()
		mu.Lock()
		for c := range conns {
			c.Close()
		}
		mu.Unlock()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		mu.Lock()
		conns[c] = true
		mu.Unlock()
		go func() {
			defer func() {
				mu.Lock()
				delete(conns, c)
				mu.Unlock()
				c.Close()
			}()
			_ = srv.Serve(ctx, c, c)
		}()
	}
}

// Bridge carries an agent's stdio (in, out) to the server on the socket at
// path, until either side ends. start, when not nil, is called once if
// nothing listens yet, to launch the app; Bridge then waits for it.
func Bridge(path string, in io.Reader, out io.Writer, start func() error) error {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil && start != nil {
		if serr := start(); serr != nil {
			return fmt.Errorf("the app isn't running and couldn't be started: %w", serr)
		}
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
			if c, err = net.DialTimeout("unix", path, time.Second); err == nil {
				break
			}
		}
	}
	if err != nil {
		return fmt.Errorf("the app isn't running (no %s): %w", path, err)
	}
	defer c.Close()
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(c, in)
		// The agent closed its end: let the app finish what it was asked.
		if uc, ok := c.(*net.UnixConn); ok {
			_ = uc.CloseWrite()
		}
		done <- err
	}()
	go func() {
		_, err := io.Copy(out, c)
		done <- err
	}()
	// The app's side ending ends the bridge; the agent's ending waits for
	// the app to answer what is in flight and close.
	err = <-done
	if err == nil {
		err = <-done
	}
	return err
}

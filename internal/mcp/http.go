package mcp

// MCP's Streamable HTTP transport, for agents on other machines or in other
// containers: one endpoint, a JSON-RPC message a POST, answered with one
// JSON object. The server sends nothing of its own accord (no progress, no
// requests to the client), so it needs no event stream: GET is refused, as
// the spec allows.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// HTTPPath is where AcceptHTTP serves the endpoint.
const HTTPPath = "/mcp"

const (
	// sessionHeader carries the session an initialize began.
	sessionHeader = "Mcp-Session-Id"
	versionHeader = "Mcp-Protocol-Version"
	// sessionIdle is how long an unused session is kept; maxSessions how
	// many are, the one unused longest giving way to a new one.
	sessionIdle = time.Hour
	maxSessions = 64
	// minToken is the shortest bearer token the endpoint takes.
	minToken = 16
)

// HTTPHandler is the endpoint: a session for each initialize, each as an
// agent's connection on the socket is.
type HTTPHandler struct {
	srv  *Server
	hash [32]byte // of the token, so that comparing takes the same time
	now  func() time.Time

	ctx   context.Context // ends every session's calls when the handler closes
	close context.CancelFunc

	mu       sync.Mutex
	sessions map[string]*httpSession
	uses     uint64 // the sessions' uses so far
}

// touch notes that s was used now. h.mu is held.
func (h *HTTPHandler) touch(s *httpSession) {
	h.uses++
	s.last, s.used = h.now(), h.uses
}

type httpSession struct {
	c    *conn
	ctx  context.Context
	end  context.CancelFunc
	last time.Time
	// used orders the sessions by when they were last used (HTTPHandler's
	// uses), which last can't: the clock may read the same for several in
	// a row (by half a millisecond on Windows), and the cap would then end
	// any of them.
	used   uint64
	active int // requests being answered
}

// HTTPHandler is the endpoint for agents that bring token as a bearer
// token. It serves whatever path it is mounted on.
func (srv *Server) HTTPHandler(token string) *HTTPHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &HTTPHandler{srv: srv, hash: sha256.Sum256([]byte(token)), now: time.Now,
		ctx: ctx, close: cancel, sessions: map[string]*httpSession{}}
}

// Close ends every session, cancelling the calls still running.
func (h *HTTPHandler) Close() {
	h.close()
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, s := range h.sessions {
		s.end()
		delete(h.sessions, id)
	}
}

// fail is an HTTP error with a JSON-RPC body, which clients show.
func fail(w http.ResponseWriter, status int, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encode(&response{ID: json.RawMessage("null"), Error: &rpcError{Code: code, Message: msg}}))
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A page the user visits could reach a listener on their machine
	// through a name it rebinds to it: a browser says where it came from.
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			fail(w, http.StatusForbidden, codeInvalidRequest, "origin not allowed")
			return
		}
	}
	auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if sum := sha256.Sum256([]byte(strings.TrimSpace(auth))); !ok || subtle.ConstantTimeCompare(sum[:], h.hash[:]) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="bowel"`)
		fail(w, http.StatusUnauthorized, codeInvalidRequest, "a bearer token is needed: run `bowel mcp token` where bowel runs")
		return
	}
	if v := r.Header.Get(versionHeader); v != "" && !slices.Contains(Versions, v) {
		fail(w, http.StatusBadRequest, codeInvalidRequest, "unsupported protocol version: "+v)
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.post(w, r)
	case http.MethodDelete:
		id := r.Header.Get(sessionHeader)
		if id == "" {
			fail(w, http.StatusBadRequest, codeInvalidRequest, "no "+sessionHeader)
			return
		}
		h.mu.Lock()
		s := h.live(id)
		if s != nil {
			s.end()
			delete(h.sessions, id)
		}
		h.mu.Unlock()
		if s == nil {
			fail(w, http.StatusNotFound, codeInvalidRequest, "no such session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		// No stream from the server: it has nothing to send unasked.
		w.Header().Set("Allow", "POST, DELETE")
		fail(w, http.StatusMethodNotAllowed, codeInvalidRequest, "POST a JSON-RPC message")
	}
}

func (h *HTTPHandler) post(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mt, _, _ := mime.ParseMediaType(ct); mt != "application/json" {
			fail(w, http.StatusUnsupportedMediaType, codeInvalidRequest, "send application/json")
			return
		}
	}
	if acc := r.Header.Get("Accept"); acc != "" && !strings.Contains(acc, "application/json") && !strings.Contains(acc, "*/*") {
		fail(w, http.StatusNotAcceptable, codeInvalidRequest, "the answer is application/json")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessage))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, codeInvalidRequest, fmt.Sprintf("a message over %d bytes", maxMessage))
			return
		}
		fail(w, http.StatusBadRequest, codeParse, err.Error())
		return
	}
	body = bytes.TrimSpace(body)
	// Batches went with 2025-06-18, and the stream transport has never
	// taken them either.
	if len(body) > 0 && body[0] == '[' {
		fail(w, http.StatusBadRequest, codeInvalidRequest, "batches are not supported: send one message a request")
		return
	}

	id := r.Header.Get(sessionHeader)
	var s *httpSession
	if id == "" {
		var m struct {
			Method string `json:"method"`
		}
		if json.Unmarshal(body, &m) != nil || m.Method != "initialize" {
			fail(w, http.StatusBadRequest, codeInvalidRequest, "no "+sessionHeader+": initialize first")
			return
		}
		ctx, end := context.WithCancel(h.ctx)
		s = &httpSession{c: h.srv.conn(), ctx: ctx, end: end}
	} else {
		h.mu.Lock()
		s = h.live(id)
		if s != nil {
			s.active++
		}
		h.mu.Unlock()
		if s == nil {
			fail(w, http.StatusNotFound, codeInvalidRequest, "no such session: initialize again")
			return
		}
		defer func() {
			h.mu.Lock()
			s.active--
			h.touch(s)
			h.mu.Unlock()
		}()
	}

	req, call, reply := s.c.message(body)
	if reply != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(encode(reply))
		return
	}
	if !call {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	// The call ends when the agent hangs up, ends the session, or the
	// server closes.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	resp := s.c.answer(ctx, req)
	if resp == nil {
		resp = &response{ID: req.ID, Error: &rpcError{Code: -32800, Message: "request cancelled"}}
	}
	if id == "" {
		if resp.Error != nil {
			s.end()
		} else if id, err = h.add(s); err != nil {
			fail(w, http.StatusServiceUnavailable, -32603, err.Error())
			return
		} else {
			w.Header().Set(sessionHeader, id)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encode(resp))
}

// live is the session id names, nil when there is none or it has been
// unused too long. h.mu is held.
func (h *HTTPHandler) live(id string) *httpSession {
	h.expire()
	return h.sessions[id]
}

// expire drops the sessions unused too long. h.mu is held.
func (h *HTTPHandler) expire() {
	now := h.now()
	for id, s := range h.sessions {
		if s.active == 0 && now.Sub(s.last) > sessionIdle {
			s.end()
			delete(h.sessions, id)
		}
	}
}

// add keeps s under a new id, making room by ending the session unused
// longest when there are too many.
func (h *HTTPHandler) add(s *httpSession) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx.Err() != nil {
		s.end()
		return "", errors.New("the server is closing")
	}
	h.expire()
	for len(h.sessions) >= maxSessions {
		oldest := ""
		for k, o := range h.sessions {
			if o.active == 0 && (oldest == "" || o.used < h.sessions[oldest].used) {
				oldest = k
			}
		}
		if oldest == "" {
			s.end()
			return "", errors.New("too many sessions busy")
		}
		h.sessions[oldest].end()
		delete(h.sessions, oldest)
	}
	h.touch(s)
	h.sessions[id] = s
	return id, nil
}

// AcceptHTTP serves the endpoint at HTTPPath on l until ctx is done, then
// ends the sessions and closes l.
func (srv *Server) AcceptHTTP(ctx context.Context, l net.Listener, token string) error {
	if len(token) < minToken {
		return fmt.Errorf("mcp: the token is shorter than %d characters", minToken)
	}
	h := srv.HTTPHandler(token)
	mux := http.NewServeMux()
	mux.Handle(HTTPPath, h)
	hs := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// No ReadTimeout or WriteTimeout: a change may wait on the user's
		// answer in a dialog for as long as they take, and a read deadline
		// passing then would cancel it. Bodies are small and come after
		// the token is checked.
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 64 << 10,
	}
	go func() {
		<-ctx.Done()
		h.Close()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if hs.Shutdown(sctx) != nil {
			_ = hs.Close()
		}
	}()
	err := hs.Serve(l)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Token is the bearer token kept in the file at path, made the first time:
// 32 random bytes, base64url, readable by this user only.
func Token(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) >= minToken {
			return t, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	// Written aside and linked into place, so that a reader never sees
	// half of it, and two starting at once agree on the one linked first.
	f, err := os.CreateTemp(filepath.Dir(path), ".mcp-token-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.WriteString(t + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o600)
	}
	if err != nil {
		return "", err
	}
	switch err := os.Link(tmp, path); {
	case errors.Is(err, os.ErrExist):
		if old, rerr := ReadToken(path); rerr == nil {
			return old, nil
		}
		// What is there is no token (an empty file): replace it.
		if err := os.Rename(tmp, path); err != nil {
			return "", err
		}
	case err != nil: // a filesystem without links
		if err := os.Rename(tmp, path); err != nil {
			return "", err
		}
	}
	return t, nil
}

// ReadToken is the token kept at path, without making one.
func ReadToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(string(b))
	if len(t) < minToken {
		return "", fmt.Errorf("%s holds no token", path)
	}
	return t, nil
}

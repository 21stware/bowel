package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMCPTLSWanted(t *testing.T) {
	for _, tc := range []struct {
		env     map[string]string
		on, own bool
		bad     bool
	}{
		{map[string]string{}, false, false, false},
		{map[string]string{mcpTLSEnv: "on"}, true, false, false},
		{map[string]string{mcpTLSEnv: "off"}, false, false, false},
		{map[string]string{mcpTLSEnv: "maybe"}, false, false, true},
		{map[string]string{mcpTLSCertEnv: "/c.pem", mcpTLSKeyEnv: "/k.pem"}, true, true, false},
		{map[string]string{mcpTLSCertEnv: "/c.pem", mcpTLSKeyEnv: "/k.pem", mcpTLSEnv: "1"}, true, true, false},
		{map[string]string{mcpTLSCertEnv: "/c.pem", mcpTLSKeyEnv: "/k.pem", mcpTLSEnv: "off"}, false, false, true},
		{map[string]string{mcpTLSCertEnv: "/c.pem"}, false, false, true},
	} {
		on, own, err := mcpTLSWanted(func(k string) string { return tc.env[k] })
		if on != tc.on || own != tc.own || (err != nil) != tc.bad {
			t.Errorf("%v: on %v own %v err %v", tc.env, on, own, err)
		}
	}
}

// The self-signed certificate is made once and kept: the same fingerprint
// at every start, a new one only for a name it lacks.
func TestMCPTLSSelfSigned(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{mcpTLSEnv: "on", mcpTLSHostsEnv: "bowel-home.lan, 192.168.1.5"}
	getenv := func(k string) string { return env[k] }

	if _, err := mcpHTTPTLS(dir, getenv, false); err == nil {
		t.Fatal("read a certificate that was never made")
	}
	tl, err := mcpHTTPTLS(dir, getenv, true)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.selfSigned || tl.from != mcpTLSFile(dir) || len(tl.fingerprint) != 32*3-1 {
		t.Fatalf("%+v", tl)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(mcpTLSFile(dir)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("the key's file: %v %v", fi.Mode(), err)
		}
	}
	leaf := tl.config.Certificates[0].Leaf
	for _, h := range []string{"localhost", "127.0.0.1", "bowel-home.lan", "192.168.1.5"} {
		if err := leaf.VerifyHostname(h); err != nil {
			t.Errorf("%s: %v", h, err)
		}
	}
	if bytes.Contains(tl.cert, []byte("PRIVATE KEY")) {
		t.Error("the certificate for clients carries the key")
	}
	// Trusted as it is, it verifies: it is its own root.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(tl.cert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "bowel-home.lan"}); err != nil {
		t.Errorf("verify: %v", err)
	}

	again, err := mcpHTTPTLS(dir, getenv, true)
	if err != nil || again.fingerprint != tl.fingerprint {
		t.Fatalf("made anew: %v %s, was %s", err, again.fingerprint, tl.fingerprint)
	}
	env[mcpTLSHostsEnv] = "bowel-home.lan nas.example.net"
	renamed, err := mcpHTTPTLS(dir, getenv, true)
	if err != nil || renamed.fingerprint == tl.fingerprint || renamed.config.Certificates[0].Leaf.VerifyHostname("nas.example.net") != nil {
		t.Fatalf("a new name kept the old certificate: %v", err)
	}
}

// writeCert makes a certificate for localhost signed by itself and writes
// it and its key to two files.
func writeCert(t *testing.T, certFile, keyFile string) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "own"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, _ := x509.MarshalPKCS8PrivateKey(key)
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		t.Fatal(err)
	}
	return der
}

// The user's own certificate is served, and a renewed one taken without
// a restart.
func TestMCPTLSUserCert(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	first := writeCert(t, certFile, keyFile)
	env := map[string]string{mcpTLSCertEnv: certFile, mcpTLSKeyEnv: keyFile}
	tl, err := mcpHTTPTLS(dir, func(k string) string { return env[k] }, true)
	if err != nil {
		t.Fatal(err)
	}
	if tl.selfSigned || tl.from != certFile || tl.fingerprint != certFingerprint(first) {
		t.Fatalf("%+v", tl)
	}
	if _, err := os.Stat(mcpTLSFile(dir)); err == nil {
		t.Error("made a certificate of its own beside the user's")
	}
	got, err := tl.config.GetCertificate(nil)
	if err != nil || !bytes.Equal(got.Certificate[0], first) {
		t.Fatalf("served: %v", err)
	}
	second := writeCert(t, certFile, keyFile)
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(certFile, later, later)
	if got, err = tl.config.GetCertificate(nil); err != nil || !bytes.Equal(got.Certificate[0], second) {
		t.Fatalf("after the renewal: %v", err)
	}
}

// mcpPost sends one JSON-RPC message to the endpoint.
func mcpPost(t *testing.T, c *http.Client, url, token, session string, msg any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return resp, out
}

// End to end: bowel serves MCP over HTTPS with its own certificate, says
// where in the data folder, and a client that trusts the certificate
// `bowel mcp cert` prints gets the gateway's tools; a remote session can't
// add a file of this machine.
func TestMCPHTTPServesTLS(t *testing.T) {
	a, _, _ := mcpApp(t)
	t.Setenv(mcpHTTPEnv, "127.0.0.1:0")
	t.Setenv(mcpTLSEnv, "on")
	t.Setenv(mcpTokenEnv, "")
	t.Setenv(mcpTLSHostsEnv, "")
	t.Cleanup(func() { mcpHTTP.Store(nil) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.serveMCPHTTP(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	var run mcpRun
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var ok bool
		if run, ok = readMCPRun(a.dataDir); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never served: %+v", mcpHTTP.Load())
		}
	}
	st := mcpHTTP.Load()
	if !run.TLS || run.Cert != mcpTLSFile(a.dataDir) || st == nil || !strings.HasPrefix(st.URL, "https://") || st.Fingerprint != run.Fingerprint {
		t.Fatalf("run %+v, state %+v", run, st)
	}
	if v := mcpHTTPView(); v["tls"] != true || v["fingerprint"] != run.Fingerprint {
		t.Errorf("get_status: %v", v)
	}

	// What `bowel mcp token` and `bowel mcp cert` say, from a shell
	// without the service's environment: the running bowel's endpoint.
	none := func(string) string { return "" }
	var out, errOut, cert bytes.Buffer
	if err := printMCPToken(&out, &errOut, a.dataDir, none, mcpTokenOpts{name: "bowel-home"}); err != nil {
		t.Fatal(err)
	}
	url := mcpClientURL(run.Listen, true)
	if !strings.Contains(out.String(), "claude mcp add --transport http bowel-home "+url) || !strings.Contains(out.String(), run.Fingerprint) ||
		!strings.Contains(out.String(), "NODE_EXTRA_CA_CERTS=bowel-home.pem") {
		t.Errorf("token:\n%s", out.String())
	}
	if err := printMCPCert(&cert, a.dataDir, none); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cert.Bytes(), []byte("PRIVATE KEY")) {
		t.Fatal("bowel mcp cert printed the key")
	}
	token, _, err := mcpHTTPToken(a.dataDir, none, false)
	if err != nil {
		t.Fatal(err)
	}

	// Not trusted: no connection.
	if _, err := http.Post(url, "application/json", strings.NewReader("{}")); err == nil {
		t.Fatal("a client that trusts nothing connected")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(cert.Bytes()) {
		t.Fatal("no certificate in bowel mcp cert's output")
	}
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	resp, init := mcpPost(t, c, url, token, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}}})
	session := resp.Header.Get("Mcp-Session-Id")
	if resp.StatusCode != 200 || session == "" {
		t.Fatalf("initialize: %d %v", resp.StatusCode, init)
	}
	if res, _ := init["result"].(map[string]any); !strings.Contains(res["instructions"].(string), "get_gateway") {
		t.Errorf("instructions: %v", res["instructions"])
	}
	_, list := mcpPost(t, c, url, token, session, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	names := map[string]bool{}
	for _, tool := range list["result"].(map[string]any)["tools"].([]any) {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	for _, n := range []string{"get_gateway", "check_dhcp", "set_gateway", "set_gateway_auto", "list_devices", "provider_add", "provider_remove"} {
		if !names[n] {
			t.Errorf("no %s over HTTP", n)
		}
	}
	_, call := mcpPost(t, c, url, token, session, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "provider_add", "arguments": map[string]any{"file": filepath.Join(a.dataDir, "settings.json")}}})
	res, _ := call["result"].(map[string]any)
	if b, _ := json.Marshal(res); res["isError"] != true || !strings.Contains(string(b), "HTTP") {
		t.Errorf("a remote agent added a file of this machine: %s", b)
	}

	cancel()
	<-done
	if _, ok := readMCPRun(a.dataDir); ok {
		t.Error("the endpoint's file outlived it")
	}
}

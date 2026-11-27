package main

// TLS for MCP over HTTP, so that the token and what the tools answer can
// cross a network that is not the user's: a VPS's, an office's. Two ways:
//
//	BOWEL_MCP_TLS_CERT, BOWEL_MCP_TLS_KEY   a certificate and its key, PEM
//	                      files of the user's own (Let's Encrypt, `tailscale
//	                      cert`, a company CA). Read again when they change,
//	                      so that a renewal needs no restart.
//	BOWEL_MCP_TLS=on      without those: a self-signed certificate, made on
//	                      the first start and kept in the data folder
//	                      (mcp-tls.pem, the key with it, this user's only).
//	                      Clients trust that one certificate (`bowel mcp
//	                      cert` prints it) or check its fingerprint, which
//	                      `bowel mcp token` prints.
//	BOWEL_MCP_TLS_HOSTS   the names and addresses clients use to reach it,
//	                      separated by commas or spaces, besides the ones the
//	                      certificate is made for anyway (localhost, the
//	                      machine's name and addresses). A client that
//	                      trusts the certificate also checks the name.
//
// Neither is on by default: a client of 0.4.9 speaks plain HTTP.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/settings"
)

const (
	mcpTLSEnv      = "BOWEL_MCP_TLS"
	mcpTLSCertEnv  = "BOWEL_MCP_TLS_CERT"
	mcpTLSKeyEnv   = "BOWEL_MCP_TLS_KEY"
	mcpTLSHostsEnv = "BOWEL_MCP_TLS_HOSTS"
	// mcpTLSYears is how long a self-signed certificate lasts. It is its
	// own root, which clients trust by itself, so no browser's limit on
	// a server certificate's life applies; a new one is a new fingerprint
	// to pin, so it should seldom be needed.
	mcpTLSYears = 10
)

// mcpTLSFile keeps the self-signed certificate and its key.
func mcpTLSFile(dataDir string) string { return filepath.Join(dataDir, "mcp-tls.pem") }

// mcpTLS is the endpoint's TLS, as it serves and as clients are told.
type mcpTLS struct {
	config *tls.Config
	// cert is the certificate clients see, PEM; Fingerprint its SHA-256.
	cert        []byte
	fingerprint string
	// from is where it is kept: the data folder's file, or the user's.
	from       string
	selfSigned bool
}

// mcpTLSWanted says whether the environment asks for TLS, and whether that
// is with the user's own certificate.
func mcpTLSWanted(getenv func(string) string) (on, own bool, err error) {
	cert, key := strings.TrimSpace(getenv(mcpTLSCertEnv)), strings.TrimSpace(getenv(mcpTLSKeyEnv))
	if (cert == "") != (key == "") {
		return false, false, fmt.Errorf("set both %s and %s, or neither", mcpTLSCertEnv, mcpTLSKeyEnv)
	}
	v := getenv(mcpTLSEnv)
	b, ok := envBool(v)
	if strings.TrimSpace(v) != "" && !ok {
		return false, false, fmt.Errorf("%s is %q: use on or off", mcpTLSEnv, v)
	}
	if cert != "" {
		if ok && !b {
			return false, false, fmt.Errorf("%s is off, and %s names a certificate: say which", mcpTLSEnv, mcpTLSCertEnv)
		}
		return true, true, nil
	}
	return b, false, nil
}

// mcpHTTPTLS is the endpoint's TLS, nil when it is plain HTTP. A
// self-signed certificate is made when create is true and there is none
// yet, or none for the names BOWEL_MCP_TLS_HOSTS asks.
func mcpHTTPTLS(dataDir string, getenv func(string) string, create bool) (*mcpTLS, error) {
	on, own, err := mcpTLSWanted(getenv)
	if err != nil || !on {
		return nil, err
	}
	if own {
		return userTLS(strings.TrimSpace(getenv(mcpTLSCertEnv)), strings.TrimSpace(getenv(mcpTLSKeyEnv)))
	}
	path := mcpTLSFile(dataDir)
	hosts := settings.SplitList(getenv(mcpTLSHostsEnv))
	pair, err := readTLSPair(path)
	switch {
	case err == nil && (!create || pair.Leaf.NotAfter.After(time.Now()) && certCovers(pair.Leaf, hosts)):
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return nil, err
	case !create:
		return nil, fmt.Errorf("no certificate in %s yet: bowel makes it when it starts with %s=on", path, mcpTLSEnv)
	default:
		if pair, err = makeTLSPair(path, mcpTLSNames(hosts, getenv)); err != nil {
			return nil, err
		}
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]})
	return &mcpTLS{
		config: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}},
		cert:   certPEM, fingerprint: certFingerprint(pair.Certificate[0]), from: path, selfSigned: true,
	}, nil
}

// userTLS serves the user's certificate, read again whenever one of the
// files changes.
func userTLS(certFile, keyFile string) (*mcpTLS, error) {
	r := &reloadingPair{cert: certFile, key: keyFile}
	pair, err := r.get()
	if err != nil {
		return nil, err
	}
	return &mcpTLS{
		config: &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return r.get() }},
		cert:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}), fingerprint: certFingerprint(pair.Certificate[0]),
		from: certFile,
	}, nil
}

// reloadingPair is a certificate in files, read again after they change.
type reloadingPair struct {
	cert, key string
	mu        sync.Mutex
	stamp     string
	pair      *tls.Certificate
}

func (r *reloadingPair) get() (*tls.Certificate, error) {
	stamp := ""
	for _, p := range []string{r.cert, r.key} {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		stamp += fmt.Sprintf("%d.%d;", fi.ModTime().UnixNano(), fi.Size())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pair != nil && stamp == r.stamp {
		return r.pair, nil
	}
	pair, err := tls.LoadX509KeyPair(r.cert, r.key)
	if err != nil {
		if r.pair != nil { // half-written by a renewal: the old one until it's whole
			return r.pair, nil
		}
		return nil, err
	}
	r.pair, r.stamp = &pair, stamp
	return r.pair, nil
}

// readTLSPair reads the certificate and key kept together in path.
func readTLSPair(path string) (tls.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return tls.Certificate{}, err
	}
	pair, err := tls.X509KeyPair(b, b)
	if err == nil && pair.Leaf == nil {
		pair.Leaf, err = x509.ParseCertificate(pair.Certificate[0])
	}
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%s: %w", path, err)
	}
	return pair, nil
}

// mcpTLSNames are the names and addresses a self-signed certificate is
// made for: localhost, this machine's name and addresses, the listen
// address's host, and hosts.
func mcpTLSNames(hosts []string, getenv func(string) string) []string {
	names := []string{"localhost", "127.0.0.1", "::1"}
	if h, err := os.Hostname(); err == nil && h != "" {
		names = append(names, h)
	}
	if host, _, err := net.SplitHostPort(mcpHTTPAddr(getenv)); err == nil && host != "" {
		if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
			names = append(names, host)
		}
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
			names = append(names, ipn.IP.String())
		}
	}
	names = append(names, hosts...)
	out := names[:0]
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// certCovers says whether c is made for every one of hosts.
func certCovers(c *x509.Certificate, hosts []string) bool {
	for _, h := range hosts {
		if strings.TrimSpace(h) != "" && c.VerifyHostname(strings.TrimSpace(h)) != nil {
			return false
		}
	}
	return true
}

// makeTLSPair makes a self-signed certificate for names and keeps it, with
// its key, in path: written aside and renamed into place, readable by this
// user only.
func makeTLSPair(path string, names []string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return tls.Certificate{}, err
	}
	host, _ := os.Hostname()
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "bowel MCP " + host, Organization: []string{"bowel"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(mcpTLSYears, 0, 0),
		// Its own root: a client that is given it trusts it as a CA.
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, n := range names {
		if ip := net.ParseIP(n); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, n)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	var buf bytes.Buffer
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = pem.Encode(&buf, &pem.Block{Type: "PRIVATE KEY", Bytes: kder})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return tls.Certificate{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".mcp-tls-*")
	if err != nil {
		return tls.Certificate{}, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.Write(buf.Bytes())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o600)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		return tls.Certificate{}, err
	}
	return readTLSPair(path)
}

// certFingerprint is a certificate's SHA-256 as openssl prints it:
// uppercase pairs of hex digits between colons.
func certFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(sum))
	for i := 0; i < len(h); i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

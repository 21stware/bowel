package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/oboo/moho/internal/i18n"
)

// directWhenProxyIsDown is the transport of the app's own downloads:
// subscriptions and rule sets. They go through the system's proxy, as a
// browser's do. When that proxy cannot be connected to at all, a browser
// stops there. bowel goes on without it: the usual case is the port an
// earlier proxy app left in the system's settings with nothing listening
// on it, and bowel cannot take that app's place before its subscription
// has come down. A proxy that answers decides, whatever it answers.
func directWhenProxyIsDown(proxied *http.Transport) http.RoundTripper {
	direct := proxied.Clone()
	direct.Proxy = nil
	return &proxyOrDirect{proxied, direct}
}

type proxyOrDirect struct{ proxied, direct *http.Transport }

func (t *proxyOrDirect) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.proxied.RoundTrip(req)
	// The transport's name for a failure to connect to the proxy itself.
	var op *net.OpError
	if err == nil || !errors.As(err, &op) || op.Op != "proxyconnect" {
		return resp, err
	}
	if req.Body != nil && req.Body != http.NoBody { // may be half read
		return nil, err
	}
	resp, derr := t.direct.RoundTrip(req)
	if derr != nil {
		addr := ""
		if u, _ := t.proxied.Proxy(req); u != nil {
			addr = u.Host
		}
		return nil, fmt.Errorf(i18n.T("系统代理 %s 连不上，直连也失败: %w", "The system proxy %s can't be reached, and connecting directly failed too: %w"), addr, derr)
	}
	return resp, nil
}

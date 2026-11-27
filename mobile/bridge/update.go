package bridge

// Looking for a newer bowel. A phone's copy came as a file from the project's
// releases and nothing updates it, so the app asks now and then whether there
// is a newer release and says where its file is. Downloading and installing
// it stay with the browser and the system.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// releasesURL is the project's releases. Its /latest redirects to the newest
// release that is neither a draft nor a prerelease, and unlike GitHub's API
// it is not counted against the address asking, which with a shared node is
// soon used up.
var releasesURL = "https://github.com/21stware/bowel/releases"

// updateTimeout is the most one way out is given to answer.
const updateTimeout = 12 * time.Second

// updateData is a newer version and the link that gets it: the Android file,
// or the release's page when that file is not there.
type updateData struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// apkArch names the release's Android file that installs here: armv7 for the
// 32-bit ARM userspace of many TV boxes, arm64 for phones. The bridge runs in
// the app's process, so GOARCH is the ABI the system installed the app for.
var apkArch = androidArch(runtime.GOARCH)

// androidArch is the Android file's name for goarch.
func androidArch(goarch string) string {
	if goarch == "arm" {
		return "armv7"
	}
	return "arm64"
}

// canSelfProxy: only Android leaves the app out of its own VPN, so only there
// do the app's requests need a way into the core. On iOS they are in the
// tunnel already, and the store updates the app.
var canSelfProxy = runtime.GOOS == "android"

// selfPort is the core's listener for the app's own requests, which the rules
// send out like any app's: GitHub is often out of reach without a node. Like
// the probe listener it takes the probe's password (lockProbe).
const selfPort = tailnetPort + 1

// selfListener is the name of the listener on selfPort.
const selfListener = "bowel-self"

// addSelfListener adds the listener on selfPort.
func addSelfListener(cfg map[string]any) {
	if !canSelfProxy {
		return
	}
	ls, _ := cfg["listeners"].([]any)
	cfg["listeners"] = append(ls, map[string]any{"name": selfListener, "type": "mixed", "listen": "127.0.0.1", "port": selfPort})
}

// updateClients are the ways out to try in turn: through the core while it
// runs, then directly, which is all there is with the VPN off and what is
// left when the core gives no answer.
func (x *bridge) updateClients() []*http.Client {
	direct := http.DefaultTransport.(*http.Transport).Clone()
	direct.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	out := []*http.Client{{Transport: direct}}
	if !canSelfProxy {
		return out
	}
	login := x.probeLogin()
	if c, _ := x.core(); c == nil || login == nil {
		return out
	}
	via := &url.URL{Scheme: "http", User: login, Host: "127.0.0.1:" + strconv.Itoa(selfPort)}
	core := http.DefaultTransport.(*http.Transport).Clone()
	core.Proxy = func(*http.Request) (*url.URL, error) { return via, nil }
	return append([]*http.Client{{Transport: core}}, out...)
}

// checkUpdate asks for the latest release; nil when current is it, or newer.
func (x *bridge) checkUpdate(ctx context.Context, current string) (up *updateData, err error) {
	for _, c := range x.updateClients() {
		up, err = latestRelease(ctx, c, current)
		c.CloseIdleConnections()
		if err == nil {
			break
		}
	}
	return up, err
}

func latestRelease(ctx context.Context, c *http.Client, current string) (*updateData, error) {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	// The answers wanted are the redirects themselves.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	to, err := redirect(ctx, c, http.MethodGet, releasesURL+"/latest")
	if err != nil {
		return nil, err
	}
	tag := path.Base(to)
	version := strings.TrimPrefix(tag, "v")
	if !strings.Contains(to, "/releases/tag/") || versionParts(version) == nil {
		return nil, fmt.Errorf("unexpected latest release: %s", to)
	}
	if !newerVersion(version, current) {
		return nil, nil
	}
	up := &updateData{Version: version, URL: releasesURL + "/tag/" + tag}
	apk := releasesURL + "/download/" + tag + "/bowel-" + version + "-android-" + apkArch + ".apk"
	if _, err := redirect(ctx, c, http.MethodHead, apk); err == nil {
		up.URL = apk
	}
	return up, nil
}

// redirect is where the address sends the request on.
func redirect(ctx context.Context, c *http.Client, method, address string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	to := resp.Header.Get("Location")
	if resp.StatusCode/100 != 3 || to == "" {
		return "", fmt.Errorf("%s: %s", address, resp.Status)
	}
	return to, nil
}

// newerVersion: both are dotted numbers and latest is the greater. Anything
// else, as a development build's name, is not compared.
func newerVersion(latest, current string) bool {
	a, b := versionParts(latest), versionParts(current)
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var m, n int
		if i < len(a) {
			m = a[i]
		}
		if i < len(b) {
			n = b[i]
		}
		if m != n {
			return m > n
		}
	}
	return false
}

func versionParts(v string) []int {
	out := []int{}
	for _, s := range strings.Split(v, ".") {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/oboo/moho/internal/mihomo"
	"gopkg.in/yaml.v3"
)

// releases stands in for the project's releases on GitHub: /latest redirects
// to the newest full release's tag, and a file of a release redirects to
// where it is stored, or is not there.
func releases(t *testing.T, tag string, apk bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/21stware/bowel/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("HEAD /download/{tag}/{file}", func(w http.ResponseWriter, r *http.Request) {
		if !apk || r.PathValue("tag") != tag {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "https://release-assets.example/"+r.PathValue("file"), http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	was := releasesURL
	releasesURL = srv.URL
	t.Cleanup(func() { releasesURL = was })
	return srv
}

func TestCheckUpdate(t *testing.T) {
	x := &bridge{}
	ctx := context.Background()

	srv := releases(t, "v0.4.5", true)
	up, err := x.checkUpdate(ctx, "0.4.4")
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.URL + "/download/v0.4.5/bowel-0.4.5-android-arm64.apk"; up == nil || up.Version != "0.4.5" || up.URL != want {
		t.Errorf("newer = %+v, want 0.4.5 at %s", up, want)
	}
	for _, current := range []string{"0.4.5", "0.5.0", "1.0.0"} {
		if up, err := x.checkUpdate(ctx, current); up != nil || err != nil {
			t.Errorf("on %s: %+v %v, want nothing", current, up, err)
		}
	}

	// A release whose Android build is not there: its page has what there is.
	srv = releases(t, "v0.4.6", false)
	up, err = x.checkUpdate(ctx, "0.4.4")
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.URL + "/tag/v0.4.6"; up == nil || up.Version != "0.4.6" || up.URL != want {
		t.Errorf("no APK = %+v, want the page %s", up, want)
	}

	// Not a version: nothing to offer, and it says so.
	releases(t, "nightly", true)
	if up, err := x.checkUpdate(ctx, "0.4.4"); up != nil || err == nil {
		t.Errorf("odd tag: %+v %v, want an error", up, err)
	}

	srv = releases(t, "v0.4.5", true)
	srv.Close()
	if up, err := x.checkUpdate(ctx, "0.4.4"); up != nil || err == nil {
		t.Errorf("offline: %+v %v, want an error", up, err)
	}
}

func TestCheckUpdateCall(t *testing.T) {
	releases(t, "v0.4.5", true)
	bMu.Lock()
	was := b
	b = &bridge{}
	bMu.Unlock()
	defer func() {
		bMu.Lock()
		b = was
		bMu.Unlock()
	}()
	if out, err := Call("checkUpdate", `{"version":"0.4.5"}`); out != "null" || err != nil {
		t.Errorf("latest: %q %v", out, err)
	}
	out, err := Call("checkUpdate", `{"version":"0.4.4"}`)
	if want := `{"version":"0.4.5","url":"` + releasesURL + `/download/v0.4.5/bowel-0.4.5-android-arm64.apk"}`; out != want || err != nil {
		t.Errorf("newer: %q %v, want %s", out, err, want)
	}
}

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"0.4.5", "0.4.4", true},
		{"0.10.0", "0.9.9", true},
		{"1.0", "0.9.9", true},
		{"0.4.4", "0.4.4", false},
		{"0.4.4", "0.4.4.1", false},
		{"0.4.4", "1.0.0", false},
		// Not versions this can compare: say nothing rather than nag.
		{"0.4.5", "", false},
		{"0.4.5", "0.4.4-debug", false},
		{"nightly", "0.4.4", false},
	} {
		if got := newerVersion(c.latest, c.current); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

// The app is outside its own VPN: with the core up the check goes out through
// the core's rules, as GitHub is often out of reach without a node, and
// straight out when that way gives no answer.
func TestUpdateThroughCore(t *testing.T) {
	was := canSelfProxy
	defer func() { canSelfProxy = was }()
	x := testBridge(t)
	type listener struct {
		Name   string `yaml:"name"`
		Listen string `yaml:"listen"`
		Port   int    `yaml:"port"`
		Proxy  string `yaml:"proxy"`
		Users  []any  `yaml:"users"`
	}
	find := func() (listener, bool) {
		t.Helper()
		if err := x.writeConfig(); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(x.dir, "config.yaml"))
		var cfg struct {
			Listeners []listener `yaml:"listeners"`
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(cfg.Listeners, func(l listener) bool { return l.Name == selfListener })
		if i < 0 {
			return listener{}, false
		}
		return cfg.Listeners[i], true
	}
	proxies := func() []*url.URL {
		t.Helper()
		r, _ := http.NewRequest(http.MethodGet, "https://github.com/21stware/bowel/releases/latest", nil)
		out := []*url.URL{}
		for _, c := range x.updateClients() {
			u, err := c.Transport.(*http.Transport).Proxy(r)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, u)
		}
		return out
	}

	canSelfProxy = false // iOS: the app's requests are in the tunnel already
	if l, ok := find(); ok {
		t.Errorf("a listener where the app needs none: %+v", l)
	}
	x.client = mihomo.NewUnix(filepath.Join(x.dir, "none.sock"))
	if got := proxies(); len(got) != 1 || got[0] != nil {
		t.Errorf("without the listener: %v, want one direct client", got)
	}
	x.client = nil

	canSelfProxy = true
	l, ok := find()
	if !ok {
		t.Fatalf("no %s listener", selfListener)
	}
	// No proxy of its own: the rules decide, as for any app's request. Any
	// app on the phone can reach it, so it takes the probe's password.
	if l.Listen != "127.0.0.1" || l.Port != selfPort || l.Proxy != "" || len(l.Users) != 1 {
		t.Errorf("listener = %+v", l)
	}
	if got := proxies(); len(got) != 1 || got[0] != nil {
		t.Errorf("VPN off: %v, want one direct client", got)
	}

	x.client = mihomo.NewUnix(filepath.Join(x.dir, "none.sock")) // the core is up
	got := proxies()
	if len(got) != 2 || got[0] == nil || got[1] != nil {
		t.Fatalf("VPN on: %v, want the core, then direct", got)
	}
	pass, _ := x.probeLogin().Password()
	if p, _ := got[0].User.Password(); got[0].Host != "127.0.0.1:"+strconv.Itoa(selfPort) || p != pass || pass == "" {
		t.Errorf("proxy = %v", got[0])
	}

	// No core is listening in this test: the answer still comes, directly.
	releases(t, "v0.4.5", true)
	up, err := x.checkUpdate(context.Background(), "0.4.4")
	if err != nil || up == nil || up.Version != "0.4.5" {
		t.Errorf("fallback: %+v %v, want 0.4.5", up, err)
	}
}

package bridge

// The core's own Tailscale (settings.Tailnet): a node of the tailnet inside
// the core, for the phone that can't run the Tailscale app beside bowel.

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// canTailnet: only Android. The iOS extension has too little memory for a
// second network stack, and iOS lets Tailscale's app run beside bowel's.
var canTailnet = runtime.GOOS == "android"

// tailnetLog is where the host keeps the core's [Tailscale] lines since the
// core last started: the core can't be asked how its node is.
const tailnetLog = "tailnet.log"

// tailnetProbe is an address only the tailnet has: dialing it through the
// node makes the core start it, which it otherwise does on first use.
const tailnetProbe = "http://100.100.100.100/"

// defaultHostname names the phone on the tailnet when the user hasn't: Go
// on Android can't read the name the phone was given.
const defaultHostname = "bowel-android"

// tailnetOverlay is what the core's config takes from the setting.
func tailnetOverlay(t settings.Tailnet) devnet.Overlay {
	if !canTailnet || !t.On {
		return devnet.Overlay{}
	}
	host := t.Hostname
	if host == "" {
		host = defaultHostname
	}
	return devnet.TsnetOverlay(devnet.Tsnet{Hostname: host, AuthKey: t.AuthKey, StateDir: tailnetStateDir(t)})
}

// tailnetStateDir is the node's state folder, relative to the core's.
func tailnetStateDir(t settings.Tailnet) string {
	if t.StateID == "" {
		return core.TsnetStateDir
	}
	return filepath.Join(core.TsnetStateDir, t.StateID)
}

// tailnetData is the node as the 设置 page shows it.
type tailnetData struct {
	// State: "off", "idle" (on, but the core hasn't started the node: it
	// does on first use), "starting", "login" (LoginURL waits), "on", or
	// "error" (Error says why).
	State    string `json:"state"`
	LoginURL string `json:"loginUrl,omitempty"`
	Error    string `json:"error,omitempty"`
	Hostname string `json:"hostname"`
}

func (x *bridge) tailnet() tailnetData {
	s := x.set.Get()
	host := s.Tailnet.Hostname
	if host == "" {
		host = defaultHostname
	}
	d := tailnetData{State: "off", Hostname: host}
	if !s.Tailnet.On {
		return d
	}
	d.State = "idle"
	if c, _ := x.core(); c == nil {
		return d
	}
	readTailnetLog(filepath.Join(x.dir, tailnetLog), &d)
	return d
}

// readTailnetLog puts in d what the lines since the node last started say.
func readTailnetLog(path string, d *tailnetData) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		_, msg, ok := strings.Cut(line, "[Tailscale](")
		if !ok {
			continue
		}
		if _, rest, ok := strings.Cut(msg, ") "); ok {
			msg = rest
		}
		msg = strings.TrimRight(msg, `"`)
		switch {
		case strings.HasPrefix(msg, "tsnet starting"):
			d.State, d.LoginURL, d.Error = "starting", "", ""
		case strings.Contains(msg, "state is NeedsLogin"):
			d.State = "login"
		case strings.Contains(msg, " or go to: "):
			_, u, _ := strings.Cut(msg, " or go to: ")
			d.State, d.LoginURL = "login", strings.TrimSpace(u)
		case strings.HasPrefix(msg, "AuthLoop: state is"), strings.HasPrefix(msg, "Authkey is set; but state is"):
			d.State, d.LoginURL, d.Error = "on", "", ""
		case strings.Contains(line, "level=error") || strings.Contains(line, "level=warning") && strings.Contains(msg, "failed"):
			d.State, d.Error = "error", msg
		}
	}
}

// tailnetLogin starts the node if it hasn't, and gives its state once it
// knows whether it needs logging in.
func (x *bridge) tailnetLogin(ctx context.Context) (tailnetData, error) {
	if !x.set.Get().Tailnet.On {
		return x.tailnet(), nil
	}
	c, err := x.core()
	if err != nil {
		return x.tailnet(), errors.New(i18n.T("先开启 VPN", "Turn the VPN on first"))
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = c.ProxyDelay(ctx, devnet.TailscaleOutbound, tailnetProbe, 15*time.Second)
	}()
	deadline := time.After(15 * time.Second)
	for {
		d := x.tailnet()
		if d.State == "on" || d.State == "error" || d.LoginURL != "" {
			return d, nil
		}
		select {
		case <-ctx.Done():
			return d, ctx.Err()
		case <-deadline:
			return d, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// tailnetLogout forgets the node: the next start logs in anew. The core
// closes a node it no longer uses only when it gets round to it, and until
// then the node may still write its state, so the next one gets a folder of
// its own and the others are removed.
func (x *bridge) tailnetLogout(ctx context.Context) error {
	var b [6]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	if err := x.change(ctx, func(s *settings.Settings) { s.Tailnet.AuthKey, s.Tailnet.StateID = "", id }); err != nil {
		return err
	}
	// What the old node said no longer holds. The host appends, so it
	// writes on from the start.
	_ = os.Truncate(filepath.Join(x.dir, tailnetLog), 0)
	root := filepath.Join(x.dir, core.TsnetStateDir)
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != id {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	return nil
}

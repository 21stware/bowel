package main

// bowel in a container: `bowel daemon` as the image's command, with the
// data folder a volume (BOWEL_DATA_DIR) and the proxy port published. A
// container has no desktop to set a system proxy on and no one at a
// window, so its first start seeds settings that serve the proxy to the
// machines around it, and its environment can say what docker-compose
// would otherwise need `docker exec` for.
//
// The environment `bowel daemon` reads, at every start:
//
//	BOWEL_CONTAINER       container mode, for any value but "", 0, false,
//	                      no or off. The first start (no settings.json in
//	                      the data folder yet) then seeds: the proxy open
//	                      to the LAN, from private addresses only
//	                      (containerLanIPs); no system proxy; no process
//	                      lookup; the language of LANG, else English. The
//	                      system proxy is refused with a reason, not
//	                      tried, and bowel reaps the orphans PID 1 inherits.
//	BOWEL_SUBSCRIPTION    subscription URLs or node links, separated by
//	                      new lines or commas: each that no Provider has
//	                      yet (by its URL, or by the link) is added, as
//	                      `bowel providers add` does. One whose download
//	                      fails is tried again a few times.
//	BOWEL_MIXED_PORT      the mixed port (1-65535).
//	BOWEL_ALLOW_LAN       whether other machines may use the proxy (a
//	                      boolean, as BOWEL_CONTAINER reads).
//	BOWEL_LAN_ALLOWED_IPS the networks they may come from, as CIDRs
//	                      separated by commas or spaces.
//
// A setting from the environment is saved, as if set by hand, and wins
// over what was set by hand since. None of them is logged with its value
// where that may carry a secret: a subscription's URL holds its token, so
// a Provider is named by its id and host.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/tun"
)

const containerEnv = "BOWEL_CONTAINER"

// containerLanIPs are who a container's proxy answers at first: the
// private networks (Docker's bridges are among them, and so is a LAN
// behind host networking) and the container itself. Not everyone: a port
// published on a server facing the internet must not be an open proxy.
var containerLanIPs = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "fc00::/7", "::1/128"}

// envBool reads a boolean from the environment; ok is false for "" and
// for what is not one.
func envBool(v string) (on, ok bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return false, false
	case "1", "true", "yes", "on", "y":
		return true, true
	case "0", "false", "no", "off", "n":
		return false, true
	}
	return false, false
}

// inContainer says whether bowel runs in container mode.
func inContainer() bool { return isContainer(os.Getenv(containerEnv)) }

func isContainer(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	on, ok := envBool(v)
	return on || !ok // any other word is a yes
}

// containerSeed is s as a container's first start has it.
func containerSeed(s *settings.Settings, lang string) {
	s.AllowLan = true
	s.LanAllowedIPs = slices.Clone(containerLanIPs)
	// No desktop: takeover by the system proxy has nothing to point.
	s.SystemProxy = false
	// The proxy's users are other machines, whose processes the core
	// can't see: looking each connection up costs and finds nothing.
	s.FindProcess = "off"
	if i18n.Known(lang) {
		s.Language = lang
	}
}

// seedContainer saves the seed into a data folder that has no settings.
func seedContainer(st *settings.Store, data string) {
	lang := savedLanguage(data) // with no file yet, the environment's
	s, err := st.Update(func(s *settings.Settings) { containerSeed(s, lang) })
	if err != nil {
		log.Println("container: seeding the settings:", err)
		return
	}
	i18n.Set(s.Language)
	log.Printf("container: first start, settings seeded: allowLan on for %s, port %d", strings.Join(s.LanAllowedIPs, " "), s.MixedPort)
}

// envSettings are the settings the environment sets, as changes to make,
// each with the line that says it; a value that is not one is a line of
// its own and no change.
func envSettings(getenv func(string) string) (changes []func(*settings.Settings), lines []string) {
	if v := strings.TrimSpace(getenv("BOWEL_MIXED_PORT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 65535 {
			changes = append(changes, func(s *settings.Settings) { s.MixedPort = n })
			lines = append(lines, "BOWEL_MIXED_PORT: port "+v)
		} else {
			lines = append(lines, "BOWEL_MIXED_PORT: not a port, ignored: "+v)
		}
	}
	if v := getenv("BOWEL_ALLOW_LAN"); strings.TrimSpace(v) != "" {
		if on, ok := envBool(v); ok {
			changes = append(changes, func(s *settings.Settings) { s.AllowLan = on })
			lines = append(lines, fmt.Sprintf("BOWEL_ALLOW_LAN: allowLan %v", on))
		} else {
			lines = append(lines, "BOWEL_ALLOW_LAN: not a boolean, ignored: "+v)
		}
	}
	if v := strings.TrimSpace(getenv("BOWEL_LAN_ALLOWED_IPS")); v != "" {
		nets := settings.SplitList(v)
		o := core.DefaultOptions.Clone()
		o.LanAllowedIPs = nets
		if err := o.Validate(); err == nil && len(nets) > 0 {
			changes = append(changes, func(s *settings.Settings) { s.LanAllowedIPs = slices.Clone(nets) })
			lines = append(lines, "BOWEL_LAN_ALLOWED_IPS: "+strings.Join(nets, " "))
		} else {
			lines = append(lines, "BOWEL_LAN_ALLOWED_IPS: ignored: "+fmt.Sprint(err))
		}
	}
	return changes, lines
}

// startContainer applies the environment before the core first starts,
// and in container mode puts away what a container has no use for.
func (a *App) startContainer() {
	changes, lines := envSettings(os.Getenv)
	if len(changes) > 0 {
		next := a.settings.Get()
		for _, c := range changes {
			c(&next)
		}
		if err := next.Validate(); err != nil {
			log.Println("env: settings not applied:", err)
		} else if _, err := a.settings.Update(func(s *settings.Settings) {
			for _, c := range changes {
				c(s)
			}
		}); err != nil {
			log.Println("env: settings not saved:", err)
		}
	}
	for _, l := range lines {
		log.Println("env:", l)
	}
	if !inContainer() {
		return
	}
	osProxy.enable = func(string, int, []string) error { return errNoSystemProxy() }
	go reapOrphans(a.stop)
	if s := a.settings.Get(); s.Tun && !s.Paused {
		if err := tun.DeviceReady(); err != nil {
			log.Println("tun:", err)
		}
	}
}

// errNoSystemProxy is what turning the system proxy on says in a container.
func errNoSystemProxy() error {
	return errors.New(i18n.T("容器里没有系统代理可设：请让程序直接使用代理端口，或用 TUN", "A container has no system proxy to set: point programs at the proxy port, or use TUN"))
}

// envSources are the entries of BOWEL_SUBSCRIPTION: split at new lines,
// and at commas where what follows starts a URL or a link, so that a
// comma inside one (a query's, a node's name) stays.
func envSources(v string) []string {
	var out []string
	for _, line := range strings.Split(v, "\n") {
		cur := ""
		for _, part := range strings.Split(line, ",") {
			if cur != "" && !schemeStart.MatchString(strings.TrimSpace(part)) {
				cur += "," + part
				continue
			}
			if t := strings.TrimSpace(cur); t != "" {
				out = append(out, t)
			}
			cur = part
		}
		if t := strings.TrimSpace(cur); t != "" {
			out = append(out, t)
		}
	}
	return out
}

var schemeStart = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// sourceHost is how a source is named in the log: a URL's host, a
// link's scheme. Never the whole of it.
func sourceHost(src string) string {
	if u, err := url.Parse(src); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		return u.Host
	}
	if i := strings.Index(src, "://"); i > 0 {
		return src[:i] + " link"
	}
	return "?"
}

// hasSource is the provider among ps that src is already: a subscription
// by its URL, a node link by the file it is kept as.
func hasSource(ps []provider.Provider, src string, content func(id string) string) (provider.Provider, bool) {
	kind, err := provider.Detect(src)
	if err != nil {
		return provider.Provider{}, false
	}
	for _, p := range ps {
		switch {
		case kind == provider.Subscription && p.Kind == provider.Subscription && p.Source == src:
			return p, true
		case kind == provider.Link && p.Kind == provider.Link && strings.TrimSpace(content(p.ID)) == src:
			return p, true
		}
	}
	return provider.Provider{}, false
}

// envRetries are the waits before a subscription that couldn't be
// added is tried again: the network may come up after the container.
var envRetries = []time.Duration{15 * time.Second, time.Minute, 5 * time.Minute}

// addEnvProviders adds BOWEL_SUBSCRIPTION's sources that are not there.
func (a *App) addEnvProviders() {
	srcs := envSources(os.Getenv("BOWEL_SUBSCRIPTION"))
	for i := 0; len(srcs) > 0; i++ {
		var left []string
		for _, src := range srcs {
			if !a.addEnvProvider(src) {
				left = append(left, src)
			}
		}
		if len(left) == 0 || i >= len(envRetries) {
			return
		}
		select {
		case <-a.stop:
			return
		case <-time.After(envRetries[i]):
		}
		srcs = left
	}
}

// addEnvProvider adds one source, and says whether it is done with: added,
// there already, or never to be (not a URL or a link).
func (a *App) addEnvProvider(src string) bool {
	host := sourceHost(src)
	kind, err := provider.Detect(src)
	if err != nil || kind == provider.File {
		log.Printf("env: BOWEL_SUBSCRIPTION: an entry (%s) is not a subscription URL or a node link, skipped", host)
		return true
	}
	content := func(id string) string {
		b, _ := os.ReadFile(a.providers.Path(id))
		return string(b)
	}
	if p, ok := hasSource(a.providers.List(), src, content); ok {
		log.Printf("env: BOWEL_SUBSCRIPTION: provider %s (%s) is there already", p.ID, host)
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	v, err := a.addProvider(ctx, src)
	if err != nil {
		log.Printf("env: BOWEL_SUBSCRIPTION: adding %s: %s", host, redact(err, src))
		return false
	}
	log.Printf("env: BOWEL_SUBSCRIPTION: added provider %s (%s)", v.ID, host)
	return true
}

// redact is err's text without src or its URL in it: a failed download
// says the URL it asked, and that holds the subscription's token.
func redact(err error, src string) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	msg := strings.ReplaceAll(err.Error(), src, "…")
	if u, perr := url.Parse(src); perr == nil && u.RawQuery != "" {
		msg = strings.ReplaceAll(msg, u.RawQuery, "…")
	}
	if u, perr := url.Parse(src); perr == nil && u.Path != "" && u.Path != "/" {
		msg = strings.ReplaceAll(msg, u.Path, "/…")
	}
	return msg
}

// healthTool answers `bowel health`: whether the core runs and its mixed
// port takes a connection, which is what a container's health check
// asks. It reads no more than that, to answer at once.
func (a *App) healthTool() mcp.Tool {
	return mcp.Tool{
		Name: "health", Title: "Health", ReadOnly: true,
		Description: "Whether the core runs and serves its mixed port: ok, and a line that says so.",
		Run: func(ctx context.Context, _ *mcp.Session, _ json.RawMessage) (any, error) {
			port := a.settings.Get().MixedPort
			cs := a.core.Status()
			listens := false
			if cs.State == core.Running {
				c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 500*time.Millisecond)
				if err == nil {
					c.Close()
					listens = true
				}
			}
			ok, text := healthText(cs, port, listens)
			return map[string]any{"ok": ok, "text": text, "core": cs.State, "port": port}, nil
		},
	}
}

// healthText is the health check's verdict and its line.
func healthText(cs core.Status, port int, listens bool) (bool, string) {
	why := strings.TrimSpace(strings.SplitN(cs.Error, "\n", 2)[0])
	switch {
	case cs.State != core.Running:
		text := fmt.Sprintf(i18n.T("内核没有运行（%s）", "The core isn't running (%s)"), cs.State)
		if why != "" {
			text += ": " + why
		}
		return false, text
	case !listens:
		text := fmt.Sprintf(i18n.T("内核在运行，但端口 %d 没有应答", "The core runs, but port %d doesn't answer"), port)
		if why != "" {
			text += ": " + why
		}
		return false, text
	}
	return true, fmt.Sprintf(i18n.T("正常：内核 %s 在运行，端口 %d", "OK: core %s running, port %d"), cs.Version, port)
}

// statZombie reads a /proc/<pid>/stat: whether the process is a zombie
// and its parent. The command's name is in parentheses and may hold
// anything, so the fields are read after the last one.
func statZombie(stat string) (zombie bool, ppid int, ok bool) {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return false, 0, false
	}
	f := strings.Fields(stat[i+1:])
	if len(f) < 2 {
		return false, 0, false
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return false, 0, false
	}
	return f[0] == "Z", ppid, true
}

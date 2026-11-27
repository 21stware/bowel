package main

// Egress for other containers and machines: what a container does with
// the extra ports and the exit lock, and the environment that sets them,
// so that a compose file alone is enough (docs/docker.md, "Egress for
// other containers").
//
// In a container every extra port is open as the mixed port is (allowLan,
// lanAllowedIps, authentication): its loopback is its own and that of the
// containers that share its network, which `-p` and the LAN never reach.
// No system proxy carries anything there, so the exit lock holds by what
// comes in, by the ports and by TUN, whether takeover is on or off: the
// core is never told to send all but the locked groups direct (Guard).
//
// The environment `bowel daemon` reads at every start, beside
// app_container.go's, each applied and saved on its own, so that one that
// is wrong leaves the others standing:
//
//	BOWEL_EXTRA_PORTS     the extra ports, as "7901=ai 7902=media" (port=
//	                      route id, separated by commas or spaces) or as
//	                      extraPorts' JSON; "none" closes them all.
//	BOWEL_LOCK            the route groups whose exit is locked, by id,
//	                      separated by commas or spaces; the others are
//	                      unlocked, unless an entry is not a group that can
//	                      be locked: a typo never loosens the lock. "none"
//	                      unlocks them all.
//	BOWEL_TUN             TUN on or off (a boolean), as `bowel tun` does.
//	BOWEL_SETTINGS        any other settings, as a JSON object by their
//	                      keys ({"sniff": true}), as `bowel settings set`
//	                      takes them; applied first, so the variables above
//	                      win. Not mcpPolicy: what agents may do is set by
//	                      hand.

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// coreOptions are the options as the core is given them: in a container,
// each extra port is open to the LAN as the mixed port is.
func coreOptions(o core.Options) core.Options {
	if !inContainer() || len(o.ExtraPorts) == 0 {
		return o
	}
	o.ExtraPorts = slices.Clone(o.ExtraPorts)
	for i := range o.ExtraPorts {
		o.ExtraPorts[i].Lan = true
	}
	return o
}

// portsHold says whether the exit lock holds by what comes in alone: in a
// container without TUN, the ports are all that brings traffic to the
// core, and the locked groups' rules are theirs.
func portsHold(s settings.Settings) bool { return inContainer() && !s.Tun }

// containerLock is the lock's entry in a container, where it is unheld
// only when TUN, which is set, carries nothing: the system proxy the
// desktop's words point to is not there.
func containerLock(e *Entry) *Entry {
	if e == nil || e.Status != "error" || !inContainer() {
		return e
	}
	e.Detail = i18n.T("TUN 没有在转发流量：与 bowel 共用网络的容器会直接连出去，从端口进来的流量仍按出口锁定走。",
		"TUN isn't carrying traffic, so containers that share bowel's network go out directly; what comes in by bowel's ports still keeps the exit lock")
	return e
}

// envChange is one setting from the environment: the change, and the
// line that says it. A value that is not one has a line and no change.
type envChange struct {
	change func(*settings.Settings)
	line   string
}

// envEgress are the egress settings the environment sets, read against
// cur, the settings as they stand.
func envEgress(getenv func(string) string, cur settings.Settings) []envChange {
	var out []envChange
	if v := strings.TrimSpace(getenv("BOWEL_EXTRA_PORTS")); v != "" {
		if ports, err := parseExtraPorts(v); err != nil {
			out = append(out, envChange{line: "BOWEL_EXTRA_PORTS: ignored: " + err.Error()})
		} else {
			out = append(out, envChange{
				change: func(s *settings.Settings) { s.ExtraPorts = slices.Clone(ports) },
				line:   "BOWEL_EXTRA_PORTS: " + describePorts(ports, cur),
			})
		}
	}
	if v := strings.TrimSpace(getenv("BOWEL_LOCK")); v != "" {
		out = append(out, envLock(v, cur))
	}
	if v := getenv("BOWEL_TUN"); strings.TrimSpace(v) != "" {
		if on, ok := envBool(v); ok {
			out = append(out, envChange{
				change: func(s *settings.Settings) {
					s.Tun = on
					if on && s.Paused { // as `bowel tun on`: only this switch goes on
						s.Paused, s.SystemProxy = false, false
					}
				},
				line: fmt.Sprintf("BOWEL_TUN: tun %v", on),
			})
		} else {
			out = append(out, envChange{line: "BOWEL_TUN: not a boolean, ignored: " + v})
		}
	}
	return out
}

// parseExtraPorts reads BOWEL_EXTRA_PORTS: extraPorts' JSON, "none", or
// port=route entries (port:route too).
func parseExtraPorts(v string) ([]core.ExtraPort, error) {
	var ports []core.ExtraPort
	switch {
	case strings.EqualFold(v, "none"):
		return []core.ExtraPort{}, nil
	case strings.HasPrefix(v, "["):
		if err := json.Unmarshal([]byte(v), &ports); err != nil {
			return nil, fmt.Errorf("not extraPorts' JSON: %v", err)
		}
	default:
		for _, f := range settings.SplitList(v) {
			i := strings.IndexAny(f, "=:")
			if i < 0 {
				return nil, fmt.Errorf("%q is not port=route", f)
			}
			n, err := strconv.Atoi(f[:i])
			if err != nil {
				return nil, fmt.Errorf("%q: %q is not a port", f, f[:i])
			}
			ports = append(ports, core.ExtraPort{Port: n, Route: f[i+1:]})
		}
	}
	if ports == nil {
		ports = []core.ExtraPort{}
	}
	o := core.DefaultOptions.Clone()
	o.ExtraPorts = ports
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return ports, nil
}

// describePorts says what the ports will be: each port and its group,
// with a word for one whose group isn't one of bowel's (closed, unless a
// provider's own routing has a group of that name).
func describePorts(ports []core.ExtraPort, cur settings.Settings) string {
	if len(ports) == 0 {
		return "none"
	}
	all := routes.Build(cur.Routing, nil)
	var parts []string
	for _, e := range ports {
		p := fmt.Sprintf("%d=%s", e.Port, e.Route)
		if cur.ImportedRouting == "" && !slices.ContainsFunc(all, func(r routes.Route) bool { return r.ID == e.Route }) {
			p += " (no such route group: closed)"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

// envLock reads BOWEL_LOCK against cur.
func envLock(v string, cur settings.Settings) envChange {
	if cur.ImportedRouting != "" {
		return envChange{line: "BOWEL_LOCK: ignored: a provider's own routing is in use, which has none of bowel's route groups"}
	}
	all := routes.Build(cur.Routing, nil)
	var lock, bad []string
	if !strings.EqualFold(v, "none") {
		for _, w := range settings.SplitList(v) {
			i := slices.IndexFunc(all, func(r routes.Route) bool { return strings.EqualFold(r.ID, w) })
			if i < 0 || !routes.Lockable(all[i].ID) {
				bad = append(bad, w)
				continue
			}
			if !slices.Contains(lock, all[i].ID) {
				lock = append(lock, all[i].ID)
			}
		}
	}
	var unlock []string
	if len(bad) == 0 {
		for _, r := range all {
			if routes.Lockable(r.ID) && !slices.Contains(lock, r.ID) {
				unlock = append(unlock, r.ID)
			}
		}
	}
	line := "BOWEL_LOCK: locked " + orNone(lock)
	if len(bad) > 0 {
		line += fmt.Sprintf("; %s: not a route group whose exit can be locked (default, direct and reject can't be), so none is unlocked", strings.Join(bad, " "))
	}
	if len(lock) == 0 && len(unlock) == 0 {
		return envChange{line: line}
	}
	return envChange{line: line, change: func(s *settings.Settings) {
		for _, id := range lock {
			s.Routing.SetLocked(id, true)
		}
		for _, id := range unlock {
			s.Routing.SetLocked(id, false)
		}
	}}
}

func orNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, " ")
}

// envSettingsJSON reads BOWEL_SETTINGS: settings by their keys, as
// `bowel settings set` takes them and with its checks, but for the ones
// that have a command or a variable of their own (tun, mode, rules…) and
// mcpPolicy, which says what agents may do. The line names the keys only:
// a value may be a password.
func (a *App) envSettingsJSON(v string) envChange {
	var changes map[string]json.RawMessage
	if err := json.Unmarshal([]byte(v), &changes); err != nil {
		return envChange{line: "BOWEL_SETTINGS: ignored: not a JSON object of settings"}
	}
	patch, err := a.settingsPatch(changes, false)
	if err != nil {
		return envChange{line: "BOWEL_SETTINGS: ignored: " + err.Error()}
	}
	keys := slices.Sorted(maps.Keys(changes))
	return envChange{line: "BOWEL_SETTINGS: " + strings.Join(keys, " "), change: func(s *settings.Settings) { _ = json.Unmarshal(patch, s) }}
}

// applyEnvEgress applies BOWEL_SETTINGS and then envEgress's settings,
// which win over it, before the core first starts, each on its own: one
// that the settings refuse (a port another one has) is logged and leaves
// the others, the lock above all, standing.
func (a *App) applyEnvEgress(getenv func(string) string) {
	if v := strings.TrimSpace(getenv("BOWEL_SETTINGS")); v != "" {
		a.applyEnv(a.envSettingsJSON(v))
	}
	for _, c := range envEgress(getenv, a.settings.Get()) {
		a.applyEnv(c)
	}
}

func (a *App) applyEnv(c envChange) {
	if c.change == nil {
		log.Println("env:", c.line)
		return
	}
	next := a.settings.Get().Clone()
	c.change(&next)
	if err := next.Validate(); err != nil {
		log.Printf("env: %s: not applied: %v", c.line, err)
		return
	}
	if _, err := a.settings.Update(c.change); err != nil {
		log.Printf("env: %s: not saved: %v", c.line, err)
		return
	}
	log.Println("env:", c.line)
}

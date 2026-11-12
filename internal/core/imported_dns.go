package core

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// DNSFill is what a config says of DNS that Options has a field for: the
// servers, which names go to which, and the names that are answered without
// asking. It is there to be copied into the settings once, when asked. How
// the resolver is run (whether it is on, its mode, where it listens, the
// fake-ip range) is never taken: that goes with how traffic is taken over.
type DNSFill struct {
	Nameservers        []string // nameserver
	DefaultNameservers []string // default-nameserver
	Fallback           []string // fallback
	ProxyNameservers   []string // proxy-server-nameserver
	DirectNameservers  []string // direct-nameserver
	NameserverPolicy   []string // nameserver-policy, as "pattern=server"
	FakeIPFilter       []string // fake-ip-filter
	Hosts              []string // hosts, as "domain=ip"
	// Skipped are the entries left out, as the config writes them: those
	// Options cannot hold, or that would stop working with bowel's own
	// routing.
	Skipped []string
}

// DNSField is one list of a DNSFill, under the config's name for it.
type DNSField struct {
	Key  string
	From []string  // the config's
	To   *[]string // the option it fills
}

// Fields pairs what f has with the options of o it goes to, the empty ones
// left out: a config that says nothing of a list leaves it as it is.
func (f DNSFill) Fields(o *Options) []DNSField {
	all := []DNSField{
		{"nameserver", f.Nameservers, &o.Nameservers},
		{"default-nameserver", f.DefaultNameservers, &o.DefaultNameservers},
		{"fallback", f.Fallback, &o.Fallback},
		{"proxy-server-nameserver", f.ProxyNameservers, &o.ProxyNameservers},
		{"direct-nameserver", f.DirectNameservers, &o.DirectNameservers},
		{"nameserver-policy", f.NameserverPolicy, &o.NameserverPolicy},
		{"fake-ip-filter", f.FakeIPFilter, &o.FakeIPFilter},
		{"hosts", f.Hosts, &o.Hosts},
	}
	return slices.DeleteFunc(all, func(d DNSField) bool { return len(d.From) == 0 })
}

// Apply puts what f has into o and says which of the config's keys changed
// something.
func (f DNSFill) Apply(o *Options) []string {
	var changed []string
	for _, d := range f.Fields(o) {
		if !slices.Equal(*d.To, d.From) {
			*d.To = slices.Clone(d.From)
			changed = append(changed, d.Key)
		}
	}
	return changed
}

// importedDNS reads the DNSFill out of a config. A `dns` that is switched
// off is read all the same: its servers are still the ones its author chose.
func importedDNS(data []byte) DNSFill {
	var f DNSFill
	var nodes map[string]yaml.Node
	if yaml.Unmarshal(data, &nodes) != nil {
		return f
	}
	// Each by itself: one that is not what it should be takes nothing of
	// the other with it.
	var top struct{ DNS, Hosts map[string]any }
	if n, ok := nodes["dns"]; ok {
		_ = n.Decode(&top.DNS)
	}
	if n, ok := nodes["hosts"]; ok {
		_ = n.Decode(&top.Hosts)
	}
	skip := func(key, entry string) { f.Skipped = append(f.Skipped, key+": "+entry) }
	servers := func(key string, keep func(string) bool) []string {
		var out []string
		for _, s := range listOf(top.DNS[key]) {
			switch {
			case s == "" || slices.Contains(out, s):
			case strings.ContainsAny(s, " \t,") || keep != nil && !keep(s):
				skip(key, s)
			default:
				out = append(out, s)
			}
		}
		return out
	}
	f.Nameservers = servers("nameserver", nil)
	f.DefaultNameservers = servers("default-nameserver", ipServer)
	f.Fallback = servers("fallback", nil)
	f.ProxyNameservers = servers("proxy-server-nameserver", nil)
	f.DirectNameservers = servers("direct-nameserver", nil)
	// A filter that names what does get a fake address means the opposite of
	// the one Options has.
	if mode, _ := top.DNS["fake-ip-filter-mode"].(string); strings.EqualFold(mode, "whitelist") {
		if len(listOf(top.DNS["fake-ip-filter"])) > 0 {
			skip("fake-ip-filter", "fake-ip-filter-mode: whitelist")
		}
	} else {
		f.FakeIPFilter = servers("fake-ip-filter", func(s string) bool { return !ruleSetKey(s) })
	}
	policy, _ := top.DNS["nameserver-policy"].(map[string]any)
	for _, pattern := range slices.Sorted(maps.Keys(policy)) {
		// A rule set is the config's own, there only while its routing is.
		if ruleSetKey(pattern) || strings.Contains(pattern, "=") {
			skip("nameserver-policy", pattern)
			continue
		}
		for _, s := range listOf(policy[pattern]) {
			if entry := pattern + "=" + s; s != "" && !slices.Contains(f.NameserverPolicy, entry) {
				f.NameserverPolicy = append(f.NameserverPolicy, entry)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(top.Hosts)) {
		to := listOf(top.Hosts[name])
		if len(to) == 0 || strings.Contains(name, "=") || strings.ContainsAny(to[0], " \t,") {
			skip("hosts", name)
			continue
		}
		// Options keeps one answer for a name.
		if len(to) > 1 {
			skip("hosts", name+" → "+strings.Join(to[1:], ", "))
		}
		f.Hosts = append(f.Hosts, name+"="+to[0])
	}
	return f
}

func ruleSetKey(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "rule-set:")
}

// listOf is a value that may be one entry or a list of them, as strings.
func listOf(v any) []string {
	switch v := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if e != nil {
				out = append(out, strings.TrimSpace(fmt.Sprint(e)))
			}
		}
		return out
	case map[string]any:
		return nil
	}
	return []string{strings.TrimSpace(fmt.Sprint(v))}
}

package routes

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// Shadow is what takes all of a rule's traffic before the rule is tried:
// a rule of a group listed above it, or the developer networks, whose
// rules go before every group's. The list's order is the only order
// (Lines), so a narrow rule below a broad one never matches anything. A
// rule of the user's inside a developer network goes before the network's
// own (core.BuildConfig), so nothing comes before it.
type Shadow struct {
	Route   string // the group whose rule comes first; "" for the developer networks
	Service string // the service holding that rule: a rule set's id for one of its lines
	Rule    string // that rule as people read it, as Rule.Shown or the set's own line
}

// Dev says it is the developer networks that come first.
func (s Shadow) Dev() bool { return s.Route == "" }

// Word says, for people, what takes all of the rule's traffic first, all
// being the routes the shadow was found in.
func (s Shadow) Word(all []Route) string {
	if s.Dev() {
		return i18n.T("不会生效：「开发者网络」排在所有路由组前面，先匹配了它", "Never matches: Developer networks, ahead of every group, take it first")
	}
	title := func(id string) string {
		for _, r := range all {
			if r.ID == id {
				return Title(r)
			}
		}
		return id
	}
	var from Service
	for _, r := range all {
		for _, sv := range r.Services {
			if sv.ID == s.Service {
				from = sv
			}
		}
	}
	if s.Route == Default && len(from.Except) > 0 { // an exception sends it on
		return fmt.Sprintf(i18n.T("不会生效：「%s / %s」的例外 %s 先把它交给「其他流量」", "Never matches: the exception %[3]s of %[1]s / %[2]s sends it to Other first"),
			title(from.Group), ServiceTitle(from), s.Rule)
	}
	path := title(s.Route)
	if from.ID != "" {
		path += " / " + ServiceTitle(from)
	}
	return fmt.Sprintf(i18n.T("不会生效：上面「%s」的 %s 先匹配", "Never matches: %s, above, takes it first by %s"), path, s.Rule)
}

// ShadowKey is the key of a rule of service in what Shadowed returns.
func ShadowKey(service, rule string) string { return service + "\x00" + rule }

// Shadowed are the rules of all that can't be reached, keyed by ShadowKey,
// each with what takes all its traffic first. dev says whether the
// developer networks take a rule's traffic (devnet.Overlay.Covers); match
// looks inside rule sets, as for ExplainSets. Either may be nil.
//
// Only what can be told for certain is: a rule taken in part isn't, nor
// one whose traffic a GeoSite or GeoIP list would take, which only the
// core can read. The default route's rules aren't either: none of them
// ever changes anything (Pointless).
func Shadowed(all []Route, dev func(text string) bool, match func(id, host, ip string) (string, bool)) map[string]Shadow {
	type above struct {
		route, service string
		rule           Rule
		set            bool
	}
	var seen []above
	out := map[string]Shadow{}
	for _, r := range Listed(all) {
		var here []above
		for _, s := range r.Services {
			for _, t := range s.ExceptLines() {
				here = append(here, above{route: Default, service: s.ID, rule: ruleOf(t)})
			}
			if liveSet(s) {
				here = append(here, above{route: r.ID, service: s.ID, set: true})
			}
			for _, l := range s.Live() {
				here = append(here, above{route: r.ID, service: s.ID, rule: l})
				if r.ID == Default {
					continue
				}
				if dev != nil && dev(l.Text) {
					if l.Builtin { // the user's go before the networks' own
						out[ShadowKey(s.ID, l.Text)] = Shadow{}
					}
					continue
				}
				for _, a := range seen {
					if a.route == r.ID { // the same exit: nothing changes
						continue
					}
					shown, ok := "", false
					if a.set {
						shown, ok = setCovers(a.service, l, match)
					} else {
						shown, ok = covers(a.rule, l)
					}
					if ok {
						out[ShadowKey(s.ID, l.Text)] = Shadow{Route: a.route, Service: a.service, Rule: shown}
						break
					}
				}
			}
		}
		seen = append(seen, here...)
	}
	return out
}

// Pointless says whether a rule of group id changes nothing: the default
// route's are tried last, right before MATCH, which sends what is left
// there anyway.
func Pointless(id string) bool { return id == Default }

// covers says whether rule a takes all the traffic of rule b, and how a
// reads.
func covers(a, b Rule) (string, bool) {
	av, bv := strings.ToLower(a.Value), strings.ToLower(b.Value)
	switch {
	case a.Kind == "" || b.Kind == "":
		return "", false
	case a.Kind == b.Kind && av == bv:
	case a.Kind == KindSuffix && (b.Kind == KindDomain || b.Kind == KindSuffix):
		if bv != av && !strings.HasSuffix(bv, "."+av) {
			return "", false
		}
	case a.Kind == KindKeyword && (b.Kind == KindDomain || b.Kind == KindSuffix || b.Kind == KindKeyword):
		if !strings.Contains(bv, av) {
			return "", false
		}
	case a.Kind == KindIP && b.Kind == KindIP:
		pa, ea := netip.ParsePrefix(a.Value)
		pb, eb := netip.ParsePrefix(b.Value)
		if ea != nil || eb != nil || pa.Bits() > pb.Bits() || !pa.Contains(pb.Addr()) {
			return "", false
		}
	default:
		return "", false
	}
	return a.Shown(), true
}

// setCovers says whether rule set id takes all the traffic of rule b, by
// its line that does: a name, a suffix of the name or a keyword in it for
// a name; a suffix or keyword for a suffix; a range holding the range for
// an address. A set's keywords can't be told against a keyword of b's.
func setCovers(id string, b Rule, match func(id, host, ip string) (string, bool)) (string, bool) {
	if match == nil {
		return "", false
	}
	switch b.Kind {
	case KindDomain:
		return match(id, strings.ToLower(b.Value), "")
	case KindSuffix:
		hit, ok := match(id, strings.ToLower(b.Value), "")
		kind, _, _ := strings.Cut(hit, ",")
		wide := kind == "DOMAIN-SUFFIX" || kind == "DOMAIN-KEYWORD" || strings.HasPrefix(hit, "+.") || strings.HasPrefix(hit, ".")
		return hit, ok && wide
	case KindIP:
		p, err := netip.ParsePrefix(b.Value)
		if err != nil {
			return "", false
		}
		hit, ok := match(id, "", p.Addr().String())
		value := hit
		if kind, rest, found := strings.Cut(hit, ","); found && strings.HasPrefix(kind, "IP-CIDR") {
			value, _, _ = strings.Cut(rest, ",")
		}
		q, err := netip.ParsePrefix(value)
		return hit, ok && err == nil && q.Bits() <= p.Bits()
	}
	return "", false
}

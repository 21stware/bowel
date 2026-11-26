package routes

import (
	"slices"
	"strings"
	"testing"
)

// withGeoLists has the core's data hold these lists while a test runs.
func withGeoLists(t *testing.T, site, ip []string) {
	t.Helper()
	old := GeoLists
	t.Cleanup(func() { GeoLists = old })
	GeoLists = func(kind string) ([]string, bool) {
		if kind == KindGeoIP {
			return ip, ip != nil
		}
		return site, site != nil
	}
}

// TestParseGeoRule: a GeoSite or GeoIP list is taken by its kind, as
// geosite:x or geoip:x, or as mihomo writes the rule; a word that is a
// GeoSite list is guessed as one; and a list the data hasn't is refused.
func TestParseGeoRule(t *testing.T) {
	withGeoLists(t, []string{"cn", "google", "netflix", "youtube", "youtube-music"}, []string{"cn", "private", "telegram"})
	for _, tc := range []struct {
		kind, text string
		want       UserRule
		bad        string // part of the error
	}{
		{KindGeo, "YouTube", UserRule{Kind: KindGeo, Value: "youtube"}, ""},
		{KindGeo, " google@cn ", UserRule{Kind: KindGeo, Value: "google@cn"}, ""},
		{"", "geosite:youtube", UserRule{Kind: KindGeo, Value: "youtube"}, ""},
		{"", "GeoSite: netflix", UserRule{Kind: KindGeo, Value: "netflix"}, ""},
		{"", "GEOSITE,youtube,Proxy", UserRule{Kind: KindGeo, Value: "youtube"}, ""},
		{KindSuffix, "GEOSITE,youtube", UserRule{Kind: KindGeo, Value: "youtube"}, ""}, // it says its kind
		{"", "youtube", UserRule{Kind: KindGeo, Value: "youtube"}, ""},                 // a word, and a list
		{KindGeoIP, "cn", UserRule{Kind: KindGeoIP, Value: "CN"}, ""},
		{"", "geoip:Private", UserRule{Kind: KindGeoIP, Value: "private"}, ""},
		{"", "GEOIP,CN,DIRECT,no-resolve", UserRule{Kind: KindGeoIP, Value: "CN"}, ""},
		{KindGeoIP, "lan", UserRule{Kind: KindGeoIP, Value: "lan"}, ""}, // the core's own, without data
		{"", "DOMAIN-SUFFIX,Google.com,auto", UserRule{Kind: KindSuffix, Value: "google.com"}, ""},
		{"", "domain,www.example.com", UserRule{Kind: KindDomain, Value: "www.example.com"}, ""},
		{"", "DOMAIN-KEYWORD,Google,PROXY", UserRule{Kind: KindKeyword, Value: "google"}, ""},
		{"", "IP-CIDR,10.1.2.0/16,DIRECT,no-resolve", UserRule{Kind: KindIP, Value: "10.1.0.0/16"}, ""},
		{"", "IP-CIDR6,2001:db8::/32,DIRECT", UserRule{Kind: KindIP, Value: "2001:db8::/32"}, ""},
		{"", "example.com:8443", UserRule{Kind: KindSuffix, Value: "example.com"}, ""}, // a port, not a kind

		{KindGeo, "youtub", UserRule{}, "youtube、youtube-music"},
		{"", "geosite:nosuch", UserRule{}, "nosuch"},
		{KindGeoIP, "us", UserRule{}, "US"},
		{KindGeo, "two words", UserRule{}, "two words"},
		{KindGeo, "@cn", UserRule{}, "@cn"},
		{KindGeoIP, "cn@x", UserRule{}, "cn@x"},
		{KindGeo, "", UserRule{}, "列表"},
		{"", "GEOSITE,", UserRule{}, "GeoSite"},
		{"", "nosuch", UserRule{}, "nosuch"}, // a word, and no list: still not a domain
		{"", "DST-PORT,80,DIRECT", UserRule{}, "DST-PORT"},
		{"", "domain-regex,^ab.*,PROXY", UserRule{}, "DOMAIN-REGEX"},
		{"", "MATCH,PROXY", UserRule{}, "MATCH"},
	} {
		got, err := ParseRule(tc.kind, tc.text, "")
		if tc.bad != "" {
			if err == nil || !strings.Contains(err.Error(), tc.bad) {
				t.Errorf("ParseRule(%q, %q) = %+v, %v; want an error with %q", tc.kind, tc.text, got, err, tc.bad)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseRule(%q, %q) = %+v, %v; want %+v", tc.kind, tc.text, got, err, tc.want)
			continue
		}
		if err := got.Valid(); err != nil {
			t.Errorf("%+v: parsed but not valid: %v", got, err)
		}
	}
}

// TestGeoRuleWithoutData: with no data to tell by, a list is taken as
// written, and a word is not guessed to be one.
func TestGeoRuleWithoutData(t *testing.T) {
	withGeoLists(t, nil, nil)
	if r, err := ParseRule(KindGeo, "anything", ""); err != nil || r.Value != "anything" || r.Unlisted() {
		t.Errorf("no data: %+v %v", r, err)
	}
	if r, err := ParseRule("", "youtube", ""); err == nil {
		t.Errorf("a word with no data: %+v", r)
	}
}

// TestPastedAppRule: PROCESS-NAME is a rule for an app only where one can
// work.
func TestPastedAppRule(t *testing.T) {
	was := AppRules
	t.Cleanup(func() { AppRules = was })
	AppRules = true
	if r, err := ParseRule("", "PROCESS-NAME,Telegram.exe,Proxy", ""); err != nil || r.Kind != KindApp || r.Value != "Telegram.exe" {
		t.Errorf("app: %+v %v", r, err)
	}
	AppRules = false
	if r, err := ParseRule("", "PROCESS-NAME,Telegram.exe", ""); err == nil {
		t.Errorf("app where apps can't be told: %+v", r)
	}
}

// TestGeoRuleIsTheBuiltIns: the user's rule for a list Moho ships reads
// as Moho's does, so adding it takes the list from its built-in service.
func TestGeoRuleIsTheBuiltIns(t *testing.T) {
	withGeoLists(t, []string{"cn", "youtube"}, []string{"cn", "private"})
	for _, tc := range []struct{ text, group, line string }{
		{"geosite:youtube", "media", "GEOSITE,youtube"},
		{"geoip:cn", "direct", "GEOIP,CN,no-resolve"},
		{"geoip:private", "direct", "GEOIP,private,no-resolve"},
	} {
		r, err := ParseRule("", tc.text, "ai")
		if err != nil {
			t.Fatal(err)
		}
		if r.Mihomo() != tc.line || !slices.Contains(builtinRules(tc.group), tc.line) {
			t.Errorf("%s: %q, and %s ships %q", tc.text, r.Mihomo(), tc.group, builtinRules(tc.group))
		}
		var p Plan
		p.Claim(nil, r)
		all := Build(p, []UserRule{r})
		if s, ok := FindRule(all, tc.line); !ok || s.Group != "ai" {
			t.Errorf("%s: claimed into %+v %v", tc.text, s, ok)
		}
	}
}

// TestUnlistedGeoRule: a saved rule whose list the data no longer has is
// still a valid rule, but doesn't reach the core, which would refuse the
// whole config over it.
func TestUnlistedGeoRule(t *testing.T) {
	withGeoLists(t, []string{"youtube"}, []string{"cn"})
	gone := UserRule{Kind: KindGeo, Value: "netflix", Route: "ai"}
	kept := UserRule{Kind: KindGeo, Value: "youtube", Route: "ai"}
	attr := UserRule{Kind: KindGeo, Value: "youtube@cn", Route: "ai"}
	ip := UserRule{Kind: KindGeoIP, Value: "US", Route: "ai"}
	if err := gone.Valid(); err != nil {
		t.Errorf("a list gone from the data makes the rule invalid: %v", err)
	}
	if !gone.Unlisted() || kept.Unlisted() || attr.Unlisted() || !ip.Unlisted() {
		t.Errorf("unlisted: %v %v %v %v", gone.Unlisted(), kept.Unlisted(), attr.Unlisted(), ip.Unlisted())
	}
	all := Build(Plan{}, nil)
	got := Active(all, []UserRule{gone, kept, attr, ip})
	if !slices.Equal(got, []UserRule{kept, attr}) {
		t.Errorf("active: %+v", got)
	}
}

// TestGeoRuleMatched: the core reports a GeoSite or GeoIP rule by its
// list, in any case.
func TestGeoRuleMatched(t *testing.T) {
	site := UserRule{Kind: KindGeo, Value: "youtube"}
	ip := UserRule{Kind: KindGeoIP, Value: "CN"}
	if !site.Matched("GeoSite", "youtube") || site.Matched("GeoIP", "youtube") || site.Matched("GeoSite", "netflix") {
		t.Error("geosite")
	}
	if !ip.Matched("GeoIP", "cn") || ip.Matched("GeoSite", "cn") {
		t.Error("geoip")
	}
	if site.Matches("www.youtube.com", "") {
		t.Error("only the core can look in a list")
	}
}

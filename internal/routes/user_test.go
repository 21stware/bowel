package routes

import (
	"slices"
	"testing"
)

func TestParseRule(t *testing.T) {
	for _, tc := range []struct {
		kind, text string
		want       UserRule
		bad        bool
	}{
		{"", "OpenAI.com", UserRule{Kind: KindSuffix, Value: "openai.com"}, false},
		{"", "  *.example.com ", UserRule{Kind: KindSuffix, Value: "example.com"}, false},
		{"", ".example.com", UserRule{Kind: KindSuffix, Value: "example.com"}, false},
		{"", "https://chat.openai.com/c/123?x=1", UserRule{Kind: KindSuffix, Value: "chat.openai.com"}, false},
		{"", "example.com:8443", UserRule{Kind: KindSuffix, Value: "example.com"}, false},
		{"", "例子.中国", UserRule{Kind: KindSuffix, Value: "xn--fsqu00a.xn--fiqs8s"}, false},
		{"", "1.2.3.4", UserRule{Kind: KindIP, Value: "1.2.3.4/32"}, false},
		{"", "10.1.2.3/8", UserRule{Kind: KindIP, Value: "10.0.0.0/8"}, false},
		{"", "2001:db8::1", UserRule{Kind: KindIP, Value: "2001:db8::1/128"}, false},
		{KindDomain, "www.example.com", UserRule{Kind: KindDomain, Value: "www.example.com"}, false},
		{KindDomain, "*.example.com", UserRule{Kind: KindSuffix, Value: "example.com"}, false},
		{KindKeyword, "Google", UserRule{Kind: KindKeyword, Value: "google"}, false},
		{KindKeyword, "a,b", UserRule{}, true},
		{KindIP, "example.com", UserRule{}, true},
		{"", "", UserRule{}, true},
		{"", "not a domain", UserRule{}, true},
		{"", "nodot", UserRule{}, true},
		{"", "-bad-.com", UserRule{}, true},
		{"", "a,b.com", UserRule{}, true},
	} {
		got, err := ParseRule(tc.kind, tc.text, "")
		if tc.bad {
			if err == nil {
				t.Errorf("ParseRule(%q, %q) = %+v, want an error", tc.kind, tc.text, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseRule(%q, %q) = %+v, %v; want %+v", tc.kind, tc.text, got, err, tc.want)
		}
		if err == nil && got.Valid() != nil {
			t.Errorf("%+v: parsed but not valid: %v", got, got.Valid())
		}
	}
}

func TestMatches(t *testing.T) {
	suffix := UserRule{Kind: KindSuffix, Value: "openai.com"}
	for host, want := range map[string]bool{"openai.com": true, "api.openai.com": true, "API.OpenAI.com.": true, "notopenai.com": false, "": false} {
		if suffix.Matches(host, "") != want {
			t.Errorf("suffix.Matches(%q) != %v", host, want)
		}
	}
	if !(UserRule{Kind: KindDomain, Value: "a.b.com"}).Matches("a.b.com", "") || (UserRule{Kind: KindDomain, Value: "a.b.com"}).Matches("x.a.b.com", "") {
		t.Error("domain")
	}
	if !(UserRule{Kind: KindKeyword, Value: "goog"}).Matches("www.google.com", "") {
		t.Error("keyword")
	}
	ip := UserRule{Kind: KindIP, Value: "10.0.0.0/8"}
	if !ip.Matches("", "10.2.3.4") || ip.Matches("", "11.0.0.1") || !ip.Matches("x.com", "::ffff:10.0.0.1") {
		t.Error("ip")
	}
}

func TestMatched(t *testing.T) {
	for _, tc := range []struct {
		r        UserRule
		typ, pay string
		want     bool
	}{
		{UserRule{Kind: KindSuffix, Value: "openai.com"}, "DomainSuffix", "openai.com", true},
		{UserRule{Kind: KindSuffix, Value: "openai.com"}, "Domain", "openai.com", false},
		{UserRule{Kind: KindDomain, Value: "a.com"}, "Domain", "a.com", true},
		{UserRule{Kind: KindKeyword, Value: "goog"}, "DomainKeyword", "goog", true},
		{UserRule{Kind: KindIP, Value: "1.2.3.0/24"}, "IPCIDR", "1.2.3.0/24", true},
		{UserRule{Kind: KindIP, Value: "2001:db8::/32"}, "IPCIDR6", "2001:db8::/32", true},
	} {
		if got := tc.r.Matched(tc.typ, tc.pay); got != tc.want {
			t.Errorf("%+v.Matched(%q, %q) = %v", tc.r, tc.typ, tc.pay, got)
		}
	}
}

func TestSite(t *testing.T) {
	for host, want := range map[string]string{
		"api.openai.com": "openai.com", "www.bbc.co.uk": "bbc.co.uk", "openai.com": "openai.com",
		"1.2.3.4": "1.2.3.4", "localhost": "localhost", "x.y.github.io": "y.github.io",
	} {
		if got := Site(host); got != want {
			t.Errorf("Site(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestUpsertRemove(t *testing.T) {
	a := UserRule{Kind: KindSuffix, Value: "a.com", Route: "ai"}
	b := UserRule{Kind: KindSuffix, Value: "b.com", Route: "dev"}
	rs := Upsert(Upsert(nil, a), b)
	if !slices.Equal(rs, []UserRule{b, a}) {
		t.Fatalf("newest first: %v", rs)
	}
	moved := a
	moved.Route = "direct"
	rs2 := Upsert(rs, moved)
	if !slices.Equal(rs2, []UserRule{b, moved}) || rs[1].Route != "ai" {
		t.Fatalf("replace in place, leaving the old slice: %v %v", rs2, rs)
	}
	if got := Remove(rs2, UserRule{Kind: KindSuffix, Value: "a.com"}); !slices.Equal(got, []UserRule{b}) {
		t.Fatalf("remove: %v", got)
	}
}

func TestExplain(t *testing.T) {
	user := []UserRule{{Kind: KindSuffix, Value: "openai.com", Route: "direct"}, {Kind: KindIP, Value: "1.1.1.0/24", Route: "dev"}}
	tpl := Template()
	if id, _, mine, sure := Explain("chat.openai.com", user, tpl); id != "direct" || !mine || !sure {
		t.Errorf("user rule first: %s %v %v", id, mine, sure)
	}
	if id, rule, mine, sure := Explain("api.anthropic.com", user, tpl); id != "ai" || mine || !sure || rule != "DOMAIN-SUFFIX,anthropic.com" {
		t.Errorf("template domain: %s %s %v %v", id, rule, mine, sure)
	}
	if id, _, _, _ := Explain("1.1.1.1", user, tpl); id != "dev" {
		t.Errorf("ip: %s", id)
	}
	if id, _, _, sure := Explain("www.youtube.com", user, tpl); id != "" || sure {
		t.Errorf("geosite is the core's to say: %s %v", id, sure)
	}
}

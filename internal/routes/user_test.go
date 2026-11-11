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

// asOn makes applications bundles, as on macOS, or not, for the test.
func asOn(t *testing.T, macOS bool) {
	old := bundles
	bundles = macOS
	t.Cleanup(func() { bundles = old })
}

// TestAppRule: a rule for an application takes a program's name or its
// path, is never guessed, and is written for the core by name, or for a
// macOS bundle by everything inside it.
func TestAppRule(t *testing.T) {
	asOn(t, true)
	for _, tc := range []struct{ text, value, mihomo string }{
		{" Telegram.exe ", "Telegram.exe", "PROCESS-NAME,Telegram.exe"},
		{`C:\Program Files\Telegram Desktop\Telegram.exe`, "Telegram.exe", "PROCESS-NAME,Telegram.exe"},
		{"/usr/lib/firefox/firefox", "firefox", "PROCESS-NAME,firefox"},
		{"com.example", "com.example", "PROCESS-NAME,com.example"},
		{"/Applications/Google Chrome.app/Contents/Frameworks/X.app/Contents/MacOS/Google Chrome Helper", "Google Chrome.app", "PROCESS-PATH-WILDCARD,*/Google Chrome.app/*"},
		{"/Applications/Claude.app/", "Claude.app", "PROCESS-PATH-WILDCARD,*/Claude.app/*"},
	} {
		r, err := ParseRule(KindApp, tc.text, "ai")
		if err != nil {
			t.Errorf("ParseRule(app, %q): %v", tc.text, err)
			continue
		}
		if r.Kind != KindApp || r.Value != tc.value || r.Route != "ai" {
			t.Errorf("ParseRule(app, %q) = %+v, want value %q", tc.text, r, tc.value)
		}
		if got := r.Mihomo(); got != tc.mihomo {
			t.Errorf("%q: Mihomo() = %q, want %q", tc.text, got, tc.mihomo)
		}
		if err := r.Valid(); err != nil {
			t.Errorf("%+v is not valid: %v", r, err)
		}
		if back := ruleOf(r.Mihomo()); back.Kind != KindApp || back.Value != r.Value {
			t.Errorf("ruleOf(%q) = %+v", r.Mihomo(), back)
		}
	}
	for _, bad := range []string{"", "  ", "a,b.exe", "/", "x\ty", ".."} {
		if r, err := ParseRule(KindApp, bad, ""); err == nil {
			t.Errorf("ParseRule(app, %q) = %+v, want an error", bad, r)
		}
	}
	// Without its kind a program's name is a domain, as it always was.
	if r, err := ParseRule("", "Code.exe", ""); err != nil || r.Kind != KindSuffix {
		t.Errorf("guessing made %+v, %v of Code.exe", r, err)
	}
	if slices.Contains(Kinds, KindApp) || !slices.Contains(AllKinds(), KindApp) {
		t.Errorf("Kinds %q are what is guessed and what the phone offers; AllKinds %q has the application", Kinds, AllKinds())
	}
	// Off macOS there are no bundles: ".app" is part of a name or a path.
	asOn(t, false)
	for text, want := range map[string]string{"viewer.app": "PROCESS-NAME,viewer.app", "/opt/viewer.app/bin/viewer": "PROCESS-NAME,viewer"} {
		if r, err := ParseRule(KindApp, text, ""); err != nil || r.Mihomo() != want {
			t.Errorf("off macOS %q is %q, %v; want %q", text, r.Mihomo(), err, want)
		}
	}
	asOn(t, true)
	// Someone else's wildcard is not an application of ours.
	if r := ruleOf("PROCESS-PATH-WILDCARD,*/bin/*"); r.Kind != "" {
		t.Errorf("ruleOf read %+v", r)
	}
}

// TestAppRuleMatches: by who connects, as the core compares: without
// regard to case, a bundle by any program inside it.
func TestAppRuleMatches(t *testing.T) {
	asOn(t, true)
	name := UserRule{Kind: KindApp, Value: "Telegram.exe"}
	bundle := UserRule{Kind: KindApp, Value: "Google Chrome.app"}
	helper := "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper"
	for _, tc := range []struct {
		r             UserRule
		process, path string
		want          bool
	}{
		{name, "telegram.exe", `C:\Apps\telegram.exe`, true},
		{name, "", `C:\Apps\Telegram.exe`, true},
		{name, "Telegram.exe.bak", "", false},
		{name, "", "", false},
		{bundle, "Google Chrome Helper", helper, true},
		{bundle, "Google Chrome Helper", "/Applications/Other.app/Contents/MacOS/Google Chrome Helper", false},
		{bundle, "Google Chrome.app", "", false},
		{UserRule{Kind: KindSuffix, Value: "telegram.exe"}, "telegram.exe", "", false},
	} {
		if got := tc.r.MatchesApp(tc.process, tc.path); got != tc.want {
			t.Errorf("%+v.MatchesApp(%q, %q) = %v", tc.r, tc.process, tc.path, got)
		}
	}
	if name.Matches("telegram.exe", "1.2.3.4") {
		t.Error("a rule for an application matched an address")
	}
	// As /connections names the rule a connection matched.
	if !name.Matched("ProcessName", "Telegram.exe") || !bundle.Matched("ProcessPathWildcard", "*/Google Chrome.app/*") {
		t.Error("the core's own name for the rule isn't recognised")
	}
	if name.Matched("ProcessName", "Other.exe") || name.Matched("DomainSuffix", "Telegram.exe") {
		t.Error("another rule is taken for this one")
	}
	if !name.Same(UserRule{Kind: KindApp, Value: "TELEGRAM.EXE", Route: "x"}) || name.Same(UserRule{Kind: KindSuffix, Value: "Telegram.exe"}) {
		t.Error("Same: an application's name goes without regard to case, within its kind")
	}
}

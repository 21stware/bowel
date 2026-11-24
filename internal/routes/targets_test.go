package routes

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	for in, want := range map[string]Target{
		"www.bilibili.com":            {"bilibili.com", "https://www.bilibili.com"},
		" http://example.com:8080/x ": {"example.com", "http://example.com:8080/x"},
		"https://a.example/204":       {"a.example", "https://a.example/204"},
	} {
		got, err := ParseTarget(in)
		if err != nil || got != want {
			t.Errorf("%q = %+v, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "ftp://x.example", "https://", "a b.example"} {
		if got, err := ParseTarget(in); err == nil {
			t.Errorf("%q accepted: %+v", in, got)
		}
	}
}

func TestValidTargets(t *testing.T) {
	a, b := Target{"a", "https://a.example"}, Target{"b", "https://b.example"}
	if err := ValidTargets("media", []Target{a, b}); err != nil {
		t.Error(err)
	}
	many := make([]Target, MaxTargets+1)
	for i := range many {
		many[i] = Target{"x", "https://x.example/" + strings.Repeat("y", i)}
	}
	for name, c := range map[string]struct {
		id   string
		list []Target
	}{
		"direct":    {"direct", []Target{a}},
		"no nodes":  {"direct", []Target{a}},
		"empty":     {"media", nil},
		"too many":  {"media", many},
		"twice":     {"media", []Target{a, a}},
		"no name":   {"media", []Target{{"", "https://a.example"}}},
		"no scheme": {"media", []Target{{"a", "a.example"}}},
	} {
		if ValidTargets(c.id, c.list) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// The sites that turn an exit away with a status of their own are tested
// for any other status; every other site is tested for any answer. The AI
// route starts on two of them, so its automatic choice leaves out the
// nodes they won't serve.
func TestExpected(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.openai.com/v1/models":     "100-402/404-599",
		"https://API.OpenAI.com":               "100-402/404-599",
		"http://api.anthropic.com:8443/x":      "100-402/404-599",
		"https://claude.ai":                    "",
		"https://openai.com":                   "",
		"https://www.gstatic.com/generate_204": "",
		"://":                                  "",
	} {
		if got := Expected(in); got != want {
			t.Errorf("Expected(%q) = %q, want %q", in, got, want)
		}
	}
	refusing := 0
	for _, s := range DefaultTargets("ai") {
		if Expected(s.URL) != "" {
			refusing++
		}
	}
	if refusing != 2 || Expected(DefaultTargets("ai")[0].URL) == "" {
		t.Errorf("the AI route's sites: %+v", DefaultTargets("ai"))
	}
	for _, id := range []string{Default, "media", "dev"} {
		for _, s := range DefaultTargets(id) {
			if Expected(s.URL) != "" {
				t.Errorf("%s: %s expects a status", id, s.URL)
			}
		}
	}
}

// Every proxied route starts with a valid list of its own.
func TestDefaultTargets(t *testing.T) {
	for _, r := range Template() {
		list := DefaultTargets(r.ID)
		if r.Direct {
			if list != nil {
				t.Errorf("%s: %+v", r.ID, list)
			}
			continue
		}
		if err := ValidTargets(r.ID, list); err != nil {
			t.Errorf("%s: %v", r.ID, err)
		}
	}
}

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUsageWarn(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	day := int64(24 * 3600)
	for _, c := range []struct {
		name string
		u    *Usage
		want string
	}{
		{"none", nil, ""},
		{"unlimited", &Usage{}, ""},
		{"far", &Usage{Expire: now.Unix() + 30*day}, ""},
		{"soon", &Usage{Expire: now.Unix() + 3*day}, Expire},
		{"ended", &Usage{Expire: now.Unix()}, Expired},
		{"ended beats spent", &Usage{Download: 10, Total: 10, Expire: now.Unix() - day}, Expired},
		{"spent", &Usage{Download: 10, Total: 10}, Spent},
		{"spent beats soon", &Usage{Download: 10, Total: 10, Expire: now.Unix() + day}, Spent},
		{"quota beats soon", &Usage{Download: 95, Total: 100, Expire: now.Unix() + day}, Quota},
		{"plenty", &Usage{Download: 50, Total: 100}, ""},
	} {
		if got := c.u.Warn(now); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if d := (&Usage{Expire: now.Unix() + 3*day - 3600}).DaysLeft(now); d != 3 {
		t.Errorf("days left %d", d)
	}
	text, _ := Reminder("机场 A", Expire, &Usage{Expire: now.Unix() + 3*day}, now)
	if !strings.Contains(text, "3") || !strings.Contains(text, "机场 A") {
		t.Errorf("reminder %q", text)
	}
}

func TestSiteURL(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":               "https://example.com",
		" https://a.example.com/x ": "https://a.example.com/x",
		"http://b.example.com":      "http://b.example.com",
		"javascript:alert(1)":       "",
		"file:///etc/passwd":        "",
		"https://user:pw@evil.com":  "",
		"ftp://example.com":         "",
		"":                          "",
		"https://":                  "",
	} {
		if got := SiteURL(in); got != want {
			t.Errorf("SiteURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSiteFromNames(t *testing.T) {
	for _, c := range []struct {
		names []string
		want  string
	}{
		{[]string{"香港 01", "剩余流量：10 GB", "官网：example.com"}, "https://example.com"},
		{[]string{"🌐 官網 https://panel.example.net/#/login"}, "https://panel.example.net/#/login"},
		// The official site is preferred to a mirror listed first.
		{[]string{"最新网址：mirror.example.org", "官网: www.example.com"}, "https://www.example.com"},
		{[]string{"发布页：t.example.io（请收藏）"}, "https://t.example.io"},
		{[]string{"套餐到期：2026-12-31", "日本 02"}, ""},
	} {
		if got := siteFromNames(c.names); got != c.want {
			t.Errorf("%q: %q, want %q", c.names, got, c.want)
		}
	}
}

// TestStoreSite: the header first, then the content's 官网 entry, then the
// subscription's host, and the content is read again once it changes.
func TestStoreSite(t *testing.T) {
	body := "proxies:\n  - {name: a, type: direct}\n"
	header := "https://panel.example.com/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if header != "" {
			w.Header().Set("Profile-Web-Page-Url", header)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(context.Background(), srv.Client(), srv.URL+"/sub?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Site(p); got != header {
		t.Errorf("from the header: %q", got)
	}
	header, body = "javascript:alert(1)", "proxies:\n  - {name: a, type: direct}\n  - {name: '官网：own.example.org', type: direct}\n"
	if p, err = s.Update(context.Background(), srv.Client(), p.ID); err != nil {
		t.Fatal(err)
	}
	if got := s.Site(p); got != "https://own.example.org" {
		t.Errorf("from the content: %q", got)
	}
	if err := os.WriteFile(s.Path(p.ID), []byte("proxies:\n  - {name: a, type: direct}\n  - {name: b, type: direct}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.Site(p); got != srv.URL || strings.Contains(got, "secret") {
		t.Errorf("from the subscription: %q, want %q", got, srv.URL)
	}
}

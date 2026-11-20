package provider

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/i18n"
)

// What Usage.Warn says, the most pressing first. Expired and Spent leave
// the provider's nodes dead, so they are said before what they cause.
const (
	Expired = "expired" // the plan has ended
	Spent   = "spent"   // all the data is used
	Quota   = "quota"   // over 90% of the data is used
	Expire  = "expire"  // the plan ends within ExpireSoon
)

// ExpireSoon is how long before its end a plan is said to be ending: long
// enough to renew while its nodes still reach the provider's site.
const ExpireSoon = 7 * 24 * time.Hour

// Warn is what a plan's usage at now warns of, "" for nothing.
func (u *Usage) Warn(now time.Time) string {
	if u == nil {
		return ""
	}
	end := time.Unix(u.Expire, 0)
	used := u.Upload + u.Download
	switch {
	case u.Expire > 0 && !now.Before(end):
		return Expired
	case u.Total > 0 && used >= u.Total:
		return Spent
	case u.Total > 0 && float64(used)/float64(u.Total) > .9:
		return Quota
	case u.Expire > 0 && end.Sub(now) < ExpireSoon:
		return Expire
	}
	return ""
}

// DaysLeft is how many days the plan has at now, rounded to the nearest
// day: 0 is within half a day. Meaningless when it doesn't expire.
func (u *Usage) DaysLeft(now time.Time) int {
	return int((time.Unix(u.Expire, 0).Sub(now) + 12*time.Hour) / (24 * time.Hour))
}

// SiteURL is s as a website to open: an http or https URL with a host,
// "https://" put before a bare domain, or "" when s is not one. What a
// provider sends is shown to the user and handed to the system to open,
// so nothing else passes.
func SiteURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.String()
}

// Reminder is what to say of provider name whose usage u warns of warn:
// a short line, and what to do about it.
func Reminder(name, warn string, u *Usage, now time.Time) (text, detail string) {
	dead := i18n.T("它的节点已经连不上了。续费后更新一下订阅就能恢复。", "Its nodes no longer connect. Renew, then update the subscription.")
	soon := i18n.T("趁节点还能用，现在就能打开官网续费，过期后可能打不开。", "Renew while its nodes still work: once it lapses, its site may not open.")
	switch warn {
	case Expired:
		return fmt.Sprintf(i18n.T("%s 已过期", "%s has expired"), name), dead
	case Spent:
		return fmt.Sprintf(i18n.T("%s 流量用完了", "%s is out of data"), name), dead
	case Quota:
		return fmt.Sprintf(i18n.T("%s 流量快用完了", "%s is almost out of data"), name), soon
	case Expire:
		if d := u.DaysLeft(now); d > 0 {
			return fmt.Sprintf(i18n.T("%s %d 天后到期", "%s expires in %d days"), name, d), soon
		}
		return fmt.Sprintf(i18n.T("%s 今天到期", "%s expires today"), name), soon
	}
	return "", ""
}

// siteIn is the address in such a name: "官网：example.com" gives
// example.com.
var siteIn = regexp.MustCompile(`(?i)(?:https?://)?(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}(?::\d{1,5})?(?:/[a-z0-9._~%!$&'()*+,;=:@/?#-]*)?`)

// siteFromNames is the website the entries that only carry information
// give ("官网：example.com"), "" when none does.
func siteFromNames(names []string) string {
	for _, re := range siteNames {
		for _, n := range names {
			loc := re.FindStringIndex(n)
			if loc == nil {
				continue
			}
			if u := SiteURL(siteIn.FindString(n[loc[1]:])); u != "" {
				return u
			}
		}
	}
	return ""
}

// Site is where to renew p: the website its subscription names, else the
// one its content's information entries give, else the subscription's own
// host, which is the provider's panel more often than not. "" for a
// provider with none of them.
func (s *Store) Site(p Provider) string {
	if p.Site != "" {
		return p.Site
	}
	if u := s.namedSite(p.ID); u != "" {
		return u
	}
	if p.Kind == Subscription {
		if u, err := url.Parse(p.Source); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

// namedSite is siteFromNames of provider id's file. The status asks for it
// every refresh while a reminder shows, so it is read again only once the
// file has changed.
func (s *Store) namedSite(id string) string {
	fi, err := os.Stat(s.Path(id))
	if err != nil {
		return ""
	}
	s.mu.Lock()
	c, ok := s.sites[id]
	s.mu.Unlock()
	if ok && c.mod.Equal(fi.ModTime()) && c.size == fi.Size() {
		return c.url
	}
	b, err := os.ReadFile(s.Path(id))
	if err != nil {
		return ""
	}
	c = siteSeen{mod: fi.ModTime(), size: fi.Size(), url: siteFromNames(allNames(b))}
	s.mu.Lock()
	if s.sites == nil {
		s.sites = map[string]siteSeen{}
	}
	s.sites[id] = c
	s.mu.Unlock()
	return c.url
}

// siteSeen is namedSite's answer for a file as it was.
type siteSeen struct {
	mod  time.Time
	size int64
	url  string
}

// allNames are the names of every entry in data, the information ones too.
func allNames(data []byte) []string {
	var doc struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
	}
	var out []string
	if yaml.Unmarshal(data, &doc) == nil && len(doc.Proxies) > 0 {
		for _, p := range doc.Proxies {
			out = append(out, p.Name)
		}
		return out
	}
	text := strings.TrimSpace(string(data))
	if !strings.Contains(text, "://") {
		if b := decodeB64(text); b != nil {
			text = string(b)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if name, _, ok := parseLink(strings.TrimSpace(line)); ok {
			out = append(out, name)
		}
	}
	return out
}

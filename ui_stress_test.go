package main

// A stress test of the interface: every language, window sizes from the
// minimum up, long texts and many items. MOHO_STRESS names a directory to
// write each frame and a report to; without it the test is skipped.
//
//	MOHO_STRESS=/tmp/stress CGO_ENABLED=0 go test -run Stress -count=1 .
//
// Each frame is checked for texts that overlap other texts, and for
// single-line texts narrower than their words (cut by an ellipsis or a
// clip), which the report lists for review.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// stressFixture is fixture() with long names, extreme numbers and many items.
func stressFixture() Snapshot {
	s := fixture()
	place := func(cc, city string, lat, lon float64) *Place {
		return &Place{Country: cc, City: city, Lat: lat, Lon: lon}
	}
	cities := []*Place{place("HK", "香港", 22.3, 114.2), place("JP", "东京", 35.7, 139.7), place("SG", "新加坡", 1.3, 103.8),
		place("US", "洛杉矶", 34.05, -118.24), place("DE", "法兰克福", 50.1, 8.7), place("GB", "伦敦", 51.5, -.1),
		place("TW", "台北", 25.0, 121.5), place("KR", "首尔", 37.5, 127.0), place("AU", "悉尼", -33.9, 151.2), nil}
	long := []string{
		"%s 01 IPLC 专线 | x3.5 倍率 | 解锁 Netflix/Disney+/HBO/ChatGPT",
		"%s-Premium-Relay-Node-%02d-CN2-GIA-Ultra-Long-Name",
		"[VIP] %s 高速中转 %02d",
		"%s %02d",
	}
	var nodes []NodeView
	provs := []string{"a", "b", "c", "d"}
	for i := 0; i < 400; i++ {
		pl := cities[i%len(cities)]
		city := "Relay"
		if pl != nil {
			city = pl.City
		}
		var name string
		switch f := long[i%len(long)]; strings.Count(f, "%") {
		case 1:
			name = fmt.Sprintf(f, city)
		default:
			name = fmt.Sprintf(f, city, i)
		}
		name = fmt.Sprintf("%s #%03d", name, i)
		delay := []int{38, 0, 9999, 120, 450, 1830}[i%6]
		src := []string{"probe", "name", "unknown"}[i%3]
		if pl == nil {
			src = "unknown"
		}
		nodes = append(nodes, NodeView{Name: name + "​‹" + provs[i%4], Display: name, Provider: provs[i%4],
			Type: []string{"Shadowsocks", "Trojan", "VMess", "Hysteria2", "VLESS", "WireGuard"}[i%6], UDP: i%2 == 0,
			Delay: delay, Tested: i%7 != 3, History: []int{delay, delay + 8, 0, delay, delay + 40}, Where: Located{Place: pl, Source: src}})
	}
	s.Nodes = nodes
	for i := range s.Routes {
		if !s.Routes[i].Direct {
			s.Routes[i].Node = nodes[i*7+1].Name // long names, some failing
		}
	}
	s.Routes[0].Auto = false
	s.Exit = &Exit{Node: nodes[1].Name, Where: nodes[1].Where}
	s.RuleCount = 123456

	longName := map[bool]string{false: "超长名字的机场提供商名称——包含了非常非常多的汉字以测试截断", true: "An Extremely Long Provider Name That Goes On And On Premium Plus"}
	var ps []ProviderView
	for i := 0; i < 40; i++ {
		pv := ProviderView{ID: fmt.Sprintf("p%02d", i), Kind: []string{"subscription", "link", "file"}[i%3], Enabled: i%5 != 4, Nodes: 10 + i*37,
			Source:    "https://very-long-subscription-host.example-airport-domain.com/api/v1/client/subscribe?token=0123456789abcdef0123456789abcdef&flag=clash",
			UpdatedAt: time.Now().Add(-time.Duration(i*7) * time.Hour), AutoUpdate: []int{0, 1, 6, 12, 24, 168}[i%6]}
		if i < 4 {
			pv.ID = provs[i]
		}
		pv.Name = longName[i%2 == 1]
		if i%3 == 2 {
			pv.Name = fmt.Sprintf("机场 %d", i)
		}
		if pv.Kind == "file" {
			pv.Source = "/Users/someone/Library/Mobile Documents/com~apple~CloudDocs/Proxies/very/deep/folder/config-with-a-long-name.yaml"
		}
		if pv.Kind == "subscription" {
			pv.Usage = &Usage{Download: int64(i+1) * 977 << 30, Upload: 12 << 30, Total: 1000 << 30, Expire: time.Now().Add(time.Duration(i-3) * 24 * time.Hour).Unix()}
		}
		switch i % 4 {
		case 1:
			pv.Warn = "quota"
		case 2:
			pv.Warn = "expire"
		}
		if i%6 == 5 {
			pv.Error = "下载订阅失败：Get \"https://very-long-subscription-host.example-airport-domain.com/api/v1/client/subscribe\": dial tcp 203.0.113.42:443: i/o timeout (Client.Timeout exceeded while awaiting headers)"
		}
		ps = append(ps, pv)
	}
	s.Providers = ps
	s.Takeover.Bypass = nil
	for i := 0; i < 60; i++ {
		s.Takeover.Bypass = append(s.Takeover.Bypass, fmt.Sprintf("*.internal-service-%02d.corp.example.com", i))
	}
	s.Takeover.Lan, s.Takeover.LanAddr = true, "192.168.100.200:7899"
	s.Settings = settings.Defaults.Clone()
	s.ConfigPath = "/Users/someone-with-a-long-name/Library/Application Support/bowel/core/config.yaml"
	s.ControllerTCP = "127.0.0.1:59999"
	s.Version = "0.1.0-dev+20261108.abcdef0123"
	s.Core.Version = "v1.19.33-alpha-smart-0123456789abcdef"
	return s
}

func stressTick(i int) Tick {
	t := Tick{ByRoute: map[string]int64{"default": 980e6, "ai": 1, "media": 512e6 + int64(i)*1e6, "dev": 0, "direct": 3e5}, Up: 120e6,
		Active: 99999, TodayDown: 9_876 << 30, TodayUp: 1_234 << 30, MonthDown: 987 << 40}
	for k := 0; k < 50; k++ {
		app := []string{"Google Chrome Helper (Renderer)", "com.docker.backend", "Visual Studio Code — Insiders", "微信", "Arc"}[k%5]
		if k >= 5 {
			app = fmt.Sprintf("%s %d", app, k) // the app's totals are by name: one row each
		}
		t.Apps = append(t.Apps, AppRate{App: app, Host: fmt.Sprintf("very-long-subdomain-%02d.cdn.some-video-streaming-platform.example.com", k),
			Route: []string{"default", "ai", "media", "dev", "direct"}[k%5], Down: int64(900e6) / int64(k+1), Up: 1e6, Conns: 9999 - k})
	}
	for k := 0; k < 40; k++ {
		t.Events = append(t.Events, Event{Time: time.Now().Add(-time.Duration(k) * time.Second),
			Text:  fmt.Sprintf("Google Chrome Helper (Renderer) → very-long-subdomain-%02d.googlevideo.com:443", k),
			Route: []string{"default", "ai", "media", "dev", "direct"}[k%5], Node: "[VIP] 洛杉矶 高速中转 02 #002", Failed: k%4 == 3})
	}
	for _, v := range t.ByRoute {
		t.Down += v
	}
	return t
}

func stressConns(n int) []Conn {
	now := time.Now()
	out := make([]Conn, n)
	for i := range out {
		st := []string{"active", "active", "closed", "failed"}[i%4]
		c := Conn{ID: fmt.Sprintf("s%05d", i), App: []string{"Google Chrome Helper (Renderer)", "com.docker.backend", "微信", "Arc", "mDNSResponder"}[i%5],
			Process: "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/131.0.6778.86/Helpers/Google Chrome Helper (Renderer).app/Contents/MacOS/Google Chrome Helper (Renderer)",
			Host:    fmt.Sprintf("rr%d---sn-very-long-subdomain-%05d.googlevideo.com", i%9, i), IP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334", Port: 65535,
			Network: []string{"tcp", "udp"}[i%2], Rule: "DOMAIN-SUFFIX", Payload: "some-really-long-rule-payload.example.com",
			Route: []string{"default", "ai", "media", "dev", "direct", ""}[i%6], Node: "[VIP] 洛杉矶 高速中转 02 #002",
			Chain: []string{"流媒体", "Relay-Chain-Outer-Hop-Name", "[VIP] 洛杉矶 高速中转 02 #002"},
			Down:  int64(i) * 9_999_999, Up: int64(i) * 999_999, DownRate: int64(n-i) * 99_999, UpRate: 1234, Start: now.Add(-time.Duration(i) * time.Minute), Status: st}
		if st == "failed" {
			c.Error = "dial tcp [2001:0db8:85a3:0000:0000:8a2e:0370:7334]:65535: connect: connection refused after 3 retries via [VIP] 洛杉矶 高速中转 02 #002"
		}
		if st != "active" {
			c.End = now
			c.DownRate, c.UpRate = 0, 0
		}
		out[i] = c
	}
	return out
}

func stressLogs(n int) []LogLine {
	base := time.Now().Add(-time.Hour)
	out := make([]LogLine, n)
	for i := range out {
		txt := fmt.Sprintf("[TCP] Google Chrome Helper (Renderer) → rr3---sn-very-long-subdomain-%05d.googlevideo.com:443 命中 DOMAIN-SUFFIX(googlevideo.com) → 流媒体[[VIP] 洛杉矶 高速中转 02 #002]", i)
		if i%10 == 0 {
			txt = strings.Repeat("averyveryverylongtokenwithoutanyspaces", 6)
		}
		out[i] = LogLine{Seq: int64(i + 1), Time: base.Add(time.Duration(i) * time.Second), Level: []string{"debug", "info", "warning", "error"}[i%4], Text: txt}
	}
	return out
}

func stressUI() *UI {
	fontsOnce.Do(registerFonts)
	u := newUI(nil)
	u.snap = stressFixture()
	for i := 0; i < 90; i++ {
		t := stressTick(i)
		u.tick = t
		u.home.pushTick(t)
	}
	return u
}

// ---- Checks ----

type finding struct{ kind, where, text, detail string }

type stressReport struct {
	t     *testing.T
	dir   string
	found []finding
	shots int
}

// fontSizes are the sizes the interface sets texts in.
var fontSizes = []float32{10, 10.5, 11, 11.5, 12, 12.5, 13, 13.5, 14, 15, 16, 17, 18, 20, 22, 24, 26, 28, 30, 34, 40, 44, 48, 56}

// cutText says whether a text's words need more room than its box: for
// every size and number of lines its box's height could hold, it does not
// fit in the box's width. It returns the width one line would need.
func cutText(text string, box ui.Rect) (bool, float32) {
	plausible, need := false, float32(0)
	for _, s := range fontSizes {
		f := ui.Font{Family: sans, Size: s, Weight: 400}
		w1, lh := ui.ShapeText(text, f, 0).Size()
		if lh > box.H+2 {
			break
		}
		for n := 1; n <= 6; n++ {
			if box.H < float32(n)*lh-1.5 || box.H > float32(n)*lh+1.5 {
				continue
			}
			plausible = true
			if n == 1 {
				if w1 <= box.W+max(3, w1*.04) {
					return false, 0
				}
				need = w1
				continue
			}
			if _, h := ui.ShapeText(text, f, box.W+3).Size(); h <= box.H+2 {
				return false, 0
			}
		}
	}
	return plausible, need
}

type lab struct {
	text string
	box  ui.Rect
}

func holdsOther(labs []lab, i int) bool {
	o := labs[i]
	for j, l := range labs {
		if j != i && contains(o.box, l.box) && l.box.W*l.box.H < o.box.W*o.box.H {
			return true
		}
	}
	return false
}

// shownText: the label is a text's own words, not a control's name for
// assistive technology, as a switch's.
func shownText(tt *ui.Tester, s string) bool {
	a, ok := tt.Access(s)
	return !ok || a.Role == ui.RoleText || a.Role == ui.RoleHeading || a.Role == ui.RoleGroup
}

func (r *stressReport) check(tt *ui.Tester, where string, w, h float32) {
	var labs []lab
	for _, s := range tt.Texts() {
		if strings.TrimSpace(s) == "" {
			continue
		}
		b, _ := tt.Find(s)
		if b.W < 1 || b.H < 1 {
			continue
		}
		labs = append(labs, lab{s, b})
	}
	seen := map[string]bool{}
	add := func(f finding) {
		k := f.kind + "|" + f.text + "|" + f.detail
		if !seen[k] {
			seen[k] = true
			r.found = append(r.found, f)
		}
	}
	for i, a := range labs {
		// A box the window's edge cuts is not the text's own.
		edge := a.box.Y+a.box.H >= h-.5 || a.box.X+a.box.W >= w-.5 || a.box.Y <= .5
		if cut, nw := cutText(a.text, a.box); cut && !edge && !holdsOther(labs, i) && shownText(tt, a.text) {
			add(finding{"cut", where, a.text, fmt.Sprintf("needs %.0f, has %.0f at x=%.0f y=%.0f", nw, a.box.W, a.box.X, a.box.Y)})
		}
		for _, b := range labs[i+1:] {
			x0, y0 := max(a.box.X, b.box.X), max(a.box.Y, b.box.Y)
			x1, y1 := min(a.box.X+a.box.W, b.box.X+b.box.W), min(a.box.Y+a.box.H, b.box.Y+b.box.H)
			if x1-x0 < 2 || y1-y0 < 3 {
				continue
			}
			inter := (x1 - x0) * (y1 - y0)
			small := min(a.box.W*a.box.H, b.box.W*b.box.H)
			if contains(a.box, b.box) || contains(b.box, a.box) {
				continue // a control's label around its text
			}
			if inter > small*.15 {
				add(finding{"overlap", where, a.text + "  ⟷  " + b.text, fmt.Sprintf("%v / %v", rectS(a.box), rectS(b.box))})
			}
		}
	}
}

func contains(o, i ui.Rect) bool {
	return i.X >= o.X-1 && i.Y >= o.Y-1 && i.X+i.W <= o.X+o.W+1 && i.Y+i.H <= o.Y+o.H+1
}

func rectS(r ui.Rect) string { return fmt.Sprintf("(%.0f,%.0f %.0f×%.0f)", r.X, r.Y, r.W, r.H) }

func (r *stressReport) snap(tt *ui.Tester, name string, w, h int) {
	r.check(tt, name, float32(w), float32(h))
	r.shots++
	shotTo(r.t, tt, r.dir, name)
}

func shotTo(t *testing.T, tt *ui.Tester, dir, name string) {
	old := os.Getenv("MOHO_SHOTS")
	os.Setenv("MOHO_SHOTS", dir)
	shot(t, tt, name)
	os.Setenv("MOHO_SHOTS", old)
}

// fail records an interaction that did not do what it should.
func (r *stressReport) fail(where, what string, args ...any) {
	r.found = append(r.found, finding{"broken", where, fmt.Sprintf(what, args...), ""})
}

func (r *stressReport) write() {
	sort.SliceStable(r.found, func(i, j int) bool { return r.found[i].kind < r.found[j].kind })
	var b strings.Builder
	fmt.Fprintf(&b, "%d frames, %d findings\n", r.shots, len(r.found))
	// Group: the same text cut on many frames is one finding.
	type key struct{ kind, text string }
	groups := map[key][]finding{}
	var order []key
	for _, f := range r.found {
		k := key{f.kind, f.text}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}
	for _, k := range order {
		fs := groups[k]
		var wh []string
		for _, f := range fs {
			wh = append(wh, f.where)
		}
		slices.Sort(wh)
		wh = slices.Compact(wh)
		if len(wh) > 6 {
			wh = append(wh[:6], fmt.Sprintf("… +%d", len(wh)-6))
		}
		fmt.Fprintf(&b, "[%s] %q  %s\n      in %s\n", k.kind, k.text, fs[0].detail, strings.Join(wh, ", "))
	}
	os.WriteFile(filepath.Join(r.dir, "report.txt"), []byte(b.String()), 0o644)
	r.t.Logf("stress: %d frames, %d findings → %s", r.shots, len(r.found), filepath.Join(r.dir, "report.txt"))
}

// TestActivityEveryLanguage: the app filter's "all apps" is saved in the
// language the interface showed, and keeps every connection in each.
func TestActivityEveryLanguage(t *testing.T) {
	defer i18n.Set("zh")
	for _, lang := range i18n.Languages {
		i18n.Set(lang)
		u := actUI()
		tt := ui.NewTester(u.view, 1060, 760)
		tt.Advance(time.Second)
		if n := len(u.actFiltered()); n != 13 {
			t.Errorf("%s: %d active connections show, want 13 (app filter %q)", lang, n, u.act.app)
		}
		if !hasTextPart(tt, "registry-1.docker.io") {
			t.Errorf("%s: the list is empty", lang)
		}
	}
}

// ---- Scenarios ----

var mainSizes = [][2]int{{960, 660}, {1060, 760}, {1440, 660}, {960, 1100}, {1920, 1200}}

func TestStress(t *testing.T) {
	dir := os.Getenv("MOHO_STRESS")
	if dir == "" {
		t.Skip("MOHO_STRESS names the directory for the stress frames")
	}
	os.MkdirAll(dir, 0o755)
	defer i18n.Set("zh")
	r := &stressReport{t: t, dir: dir}
	defer r.write()
	only := os.Getenv("MOHO_STRESS_ONLY") // a scenario's prefix, to run just it
	run := func(name string, f func()) {
		if only == "" || strings.HasPrefix(name, only) {
			f()
		}
	}
	for _, lang := range i18n.Languages {
		if l := os.Getenv("MOHO_STRESS_LANG"); l != "" && l != lang {
			continue
		}
		i18n.Set(lang)
		for _, sz := range mainSizes {
			w, h := sz[0], sz[1]
			tag := fmt.Sprintf("%s-%dx%d", lang, w, h)
			run("home", func() { stressHome(r, tag, w, h) })
			run("activity", func() { stressActivity(r, tag, w, h) })
			run("providers", func() { stressProviders(r, tag, w, h) })
		}
		for _, sz := range [][2]int{{680, 440}, {760, 560}, {1200, 900}} {
			run("settings", func() { stressSettings(r, fmt.Sprintf("%s-%dx%d", lang, sz[0], sz[1]), sz[0], sz[1]) })
		}
		for _, h := range []int{480, 820} {
			run("panel", func() { stressPanel(r, fmt.Sprintf("%s-320x%d", lang, h), h) })
		}
	}
}

func stressHome(r *stressReport, tag string, w, h int) {
	u := stressUI()
	u.snap.State = StateDegraded
	u.snap.Issue = &Issue{Level: "warn", Text: i18n.T("3 个节点连续失败，默认路由已自动切换到备用节点", "3 nodes keep failing; the default route switched to a backup node by itself"),
		Detail: strings.Repeat("detail ", 30), Action: "activity", Label: i18n.T("查看日志", "Show log")}
	tt := ui.NewTester(u.view, w, h)
	tt.Advance(time.Second)
	r.snap(tt, "home-"+tag, w, h)
	for _, id := range []string{"hero", "map", "mode", "takeover", "routes", "speed", "apps", "events"} {
		u.home.x, u.home.sub = id, ""
		tt.Frame()
		tt.Advance(2 * time.Second)
		r.snap(tt, "home-x-"+id+"-"+tag, w, h)
	}
	// The node picker with 400 nodes: scroll to the end, the last node
	// must show and take a click.
	u.home.x, u.home.sub = "routes", "ai"
	tt.Frame()
	tt.Advance(2 * time.Second)
	r.snap(tt, "home-x-routes-ai-"+tag, w, h)
	// The picker ranks nodes by latency: its last row is the last of that.
	ranked := slices.Clone(u.snap.Nodes)
	sort.SliceStable(ranked, func(i, j int) bool { return rankOf(ranked[i]) < rankOf(ranked[j]) })
	last := ranked[len(ranked)-1].Display
	// Wheel over the list just under its toolbar: a node's name may be
	// the exit's too, and then Find lands on the header instead.
	if box, ok := tt.Find(i18n.T("全部测速", "Test all")); ok {
		for i := 0; i < 400 && !visible(tt, last, w, h); i++ {
			tt.Wheel(box.X-120, box.Y+box.H+80, 0, 120) // under a row's height of the shortest list
			tt.Advance(100 * time.Millisecond)
		}
	}
	if !visible(tt, last, w, h) {
		r.fail("home-x-routes-ai-"+tag, "the node picker never shows its last node %q", last)
	} else {
		r.snap(tt, "home-x-routes-ai-end-"+tag, w, h)
		// Right-click: the same check that the row takes the pointer,
		// without choosing the node, which needs a core.
		if err := tt.RightClick(last); err != nil {
			r.fail("home-x-routes-ai-end-"+tag, "last node: %v", err)
		}
		tt.CloseMenu()
	}
	// Off: the board's headline and its button.
	u.home.x = ""
	u.snap.Takeover.On, u.snap.State, u.snap.Issue = false, StateOff, nil
	tt.Frame()
	tt.Advance(2 * time.Second)
	r.snap(tt, "home-off-"+tag, w, h)
	// The tabs take clicks in every language.
	for _, pg := range pages {
		if err := tt.Click(i18n.T(pg.zh, pg.en)); err != nil {
			r.fail("tabs-"+tag, "tab %s: %v", pg.id, err)
		} else if tt.Frame(); u.page != pg.id {
			r.fail("tabs-"+tag, "tab %s shows %s", pg.id, u.page)
		}
	}
}

// visible says whether a text shows inside the window.
func visible(tt *ui.Tester, s string, w, h int) bool {
	b, ok := tt.Find(s)
	return ok && b.W > 0 && b.H > 0 && b.Y >= 0 && b.Y+b.H <= float32(h)
}

func stressActivity(r *stressReport, tag string, w, h int) {
	u := stressUI()
	u.page = "activity"
	u.act.apply(stressConns(3000), stressLogs(5000))
	tt := ui.NewTester(u.view, w, h)
	tt.Advance(time.Second)
	r.snap(tt, "activity-"+tag, w, h)
	u.act.status = slices.Index(actStatuses, "all")
	tt.Frame()
	tt.Advance(time.Second)
	r.snap(tt, "activity-all-"+tag, w, h)
	// Scroll to the end of 3000 rows; the last row opens its detail.
	rows := u.actFiltered()
	lastHost := rows[len(rows)-1].Host
	firstHost := rows[0].Host
	if box, ok := findPart(tt, firstHost); ok {
		for i := 0; i < 2000 && !visiblePart(tt, lastHost, h); i++ {
			tt.Wheel(box.X+box.W/2, box.Y+box.H/2, 0, 2000)
			tt.Advance(50 * time.Millisecond)
		}
	}
	if !visiblePart(tt, lastHost, h) {
		r.fail("activity-all-"+tag, "the list never shows its last row %q", lastHost)
	} else {
		r.snap(tt, "activity-end-"+tag, w, h)
		b, _ := findPart(tt, lastHost)
		tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
		tt.Advance(time.Second)
		if u.act.open == "" {
			r.fail("activity-end-"+tag, "clicking the last row opens no detail")
		} else if !visible(tt, i18n.T("网络", "Network"), w, h) {
			r.fail("activity-detail-"+tag, "the last row's detail opens out of sight")
		}
		r.snap(tt, "activity-detail-"+tag, w, h)
	}
	u.act.open = ""
	u.act.status = 2 // failed: long errors
	tt.Frame()
	tt.Advance(time.Second)
	r.snap(tt, "activity-failed-"+tag, w, h)
	// Logs, 5000 lines, some one long word.
	u.act.logTab = true
	tt.Frame()
	tt.Advance(time.Second)
	r.snap(tt, "activity-logs-"+tag, w, h)
}

func visiblePart(tt *ui.Tester, s string, h int) bool {
	b, ok := findPart(tt, s)
	return ok && b.H > 0 && b.Y >= 0 && b.Y+b.H <= float32(h)
}

func stressProviders(r *stressReport, tag string, w, h int) {
	u := stressUI()
	u.page = "providers"
	tt := ui.NewTester(u.view, w, h)
	tt.Advance(time.Second)
	r.snap(tt, "providers-"+tag, w, h)
	// The last of 40 providers: reachable, and it opens.
	last := u.snap.Providers[len(u.snap.Providers)-1]
	first := u.snap.Providers[0]
	if box, ok := tt.Find(first.Name); ok {
		for i := 0; i < 200 && !visible(tt, last.Name, w, h); i++ {
			tt.Wheel(box.X+box.W/2, box.Y+box.H/2, 0, 600)
			tt.Advance(100 * time.Millisecond)
		}
	}
	if !visible(tt, last.Name, w, h) {
		r.fail("providers-"+tag, "the list never shows its last provider %q", last.Name)
	} else {
		r.snap(tt, "providers-end-"+tag, w, h)
	}
	// One with many nodes opened.
	u.prov.open = "a"
	tt.Frame()
	tt.Advance(time.Second)
	r.snap(tt, "providers-open-"+tag, w, h)
	// The add bar with a long paste.
	u.prov.open = ""
	u.prov.input = strings.Repeat("vmess://eyJhZGQiOiJ2ZXJ5LWxvbmctaG9zdC5leGFtcGxlLmNvbSJ9\n", 30)
	tt.Frame()
	tt.Advance(time.Second)
	r.snap(tt, "providers-add-"+tag, w, h)
}

func stressSettings(r *stressReport, tag string, w, h int) {
	u := stressUI()
	tt := ui.NewTester(u.settingsView, w, h)
	tt.Advance(time.Second)
	for _, sec := range settingsSections {
		if err := tt.Click(sec.name()); err != nil {
			r.fail("settings-"+tag, "section %s: %v", sec.id, err)
			continue
		}
		tt.Frame()
		tt.Advance(500 * time.Millisecond)
		if u.set.section != sec.id {
			r.fail("settings-"+tag, "clicking %s chose %q", sec.name(), u.set.section)
		}
		r.snap(tt, "settings-"+sec.id+"-"+tag, w, h)
	}
}

func stressPanel(r *stressReport, tag string, h int) {
	u := stressUI()
	u.snap.State = StateDegraded
	u.snap.Issue = &Issue{Level: "warn", Text: i18n.T("3 个节点连续失败，默认路由已自动切换到备用节点", "3 nodes keep failing; the default route switched to a backup node by itself"), Action: "activity", Label: i18n.T("查看日志", "Show log")}
	tt := ui.NewTester(u.panelView, panelWidth, h)
	tt.Advance(time.Second)
	r.snap(tt, "panel-"+tag, panelWidth, h)
}

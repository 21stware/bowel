package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/speed"
)

// The node list gets a download column once a node has been tested, and
// the button asks first, saying what it may cost.
func TestBandwidthInNodeList(t *testing.T) {
	for _, dark := range []bool{false, true} {
		suffix := map[bool]string{false: "", true: "-dark"}[dark]
		u := fixtureUI()
		tt := ui.NewTesterWith(u.view, 1060, 1000, ui.TesterOptions{Dark: dark})
		u.home.x, u.home.sub = "routes", "media"
		tt.Frame()
		tt.Advance(2 * time.Second)
		if !tt.HasText("测下载") {
			t.Fatalf("no download button: %q", tt.Texts())
		}
		if slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasSuffix(s, "Mbps") }) {
			t.Error("a download column before any test")
		}

		now := time.Now()
		for i, mbps := range []float64{64.6, 188, 0} {
			u.snap.Nodes[i].Download = &DownloadTest{Mbps: mbps, Failed: mbps == 0, At: now}
		}
		tt.Frame()
		tt.Advance(time.Second)
		for _, s := range []string{"下载", "65 Mbps", "188 Mbps", "失败"} {
			if !tt.HasText(s) {
				t.Errorf("no %q: %q", s, tt.Texts())
			}
		}
		if tt.HasText("测出来的下载速度都差不多：瓶颈可能在本地网络，换节点帮助不大。") {
			t.Error("65 against 188 Mbps read as alike")
		}
		shot(t, tt, "bandwidth-column"+suffix)

		u.home.bwPlan = bandwidthPlan{Nodes: []string{u.snap.Nodes[0].Name, u.snap.Nodes[1].Name, u.snap.Nodes[5].Name}, MaxBytes: 3 * speed.DownloadCap, Cap: speed.DownloadCap, URL: "https://speed.cloudflare.com/__down?bytes=50000000"}
		u.home.bwOpen = true
		tt.Frame()
		tt.Advance(time.Second)
		for _, s := range []string{"测流媒体的下载速度", "开始测试", "取消", "当前"} {
			if !tt.HasText(s) {
				t.Errorf("dialog lacks %q: %q", s, tt.Texts())
			}
		}
		shot(t, tt, "bandwidth-dialog"+suffix)
	}
}

// 测下载 takes its nodes from the rows the list's filters let through: the
// range of 自动最快, the region chosen in the others, and the search (issue
// 267).
func TestBandwidthOfTheRowsShown(t *testing.T) {
	u := fixtureUI()
	tt := ui.NewTester(u.view, 1060, 1000)
	u.home.x, u.home.sub = "routes", "media"
	tt.Frame()
	tt.Advance(2 * time.Second)
	var hk []string
	for _, n := range u.snap.Nodes {
		if n.Where.Place != nil && n.Where.Place.Country == "HK" {
			hk = append(hk, n.Name)
		}
	}
	slices.Sort(hk)
	// planned opens the dialog, reads its plan and closes it again.
	planned := func(why string) []string {
		t.Helper()
		tt.Frame()
		tt.Advance(time.Second)
		if err := tt.Click("测下载"); err != nil {
			t.Fatalf("%s: %v", why, err)
		}
		tt.Advance(time.Second)
		got := slices.Sorted(slices.Values(u.home.bwPlan.Nodes))
		u.home.bwOpen = false
		tt.Frame()
		tt.Advance(time.Second)
		return got
	}

	// 自动最快 with its range set to HK.
	if err := tt.Click("HK 3"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := planned("range"); !slices.Equal(got, hk) {
		t.Errorf("with the range HK the test takes %q, want %q", got, hk)
	}

	// 固定节点, the list filtered to HK: the range is 自动最快's alone.
	if err := tt.Click("HK 3"); err != nil { // the range back to every region
		t.Fatal(err)
	}
	if err := tt.Click("固定节点"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := planned("fixed, every region"); len(got) != len(u.snap.Nodes) {
		t.Errorf("with no region chosen the test takes %d of %d nodes: %q", len(got), len(u.snap.Nodes), got)
	}
	if err := tt.Click("HK"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := planned("fixed, HK"); !slices.Equal(got, hk) {
		t.Errorf("with HK chosen the test takes %q, want %q", got, hk)
	}
	u.home.bwOpen = true
	tt.Frame()
	tt.Advance(time.Second)
	shot(t, tt, "bandwidth-region")
	u.home.bwOpen = false

	// And a search among them.
	u.home.query = "iplc"
	if got := planned("search"); len(got) != 1 || display(got[0]) != "香港 03 IPLC" {
		t.Errorf("searched for iplc among HK, the test takes %q", got)
	}
}

// With every rate about the same, the list says the line is the limit.
func TestBandwidthAlikeNote(t *testing.T) {
	u := fixtureUI()
	tt := ui.NewTester(u.view, 1060, 1000)
	u.home.x, u.home.sub = "routes", "media"
	now := time.Now()
	for i, mbps := range []float64{62, 65, 68} {
		u.snap.Nodes[i].Download = &DownloadTest{Mbps: mbps, At: now}
	}
	tt.Frame()
	tt.Advance(2 * time.Second)
	if !tt.HasText("测出来的下载速度都差不多：瓶颈可能在本地网络，换节点帮助不大。") {
		t.Errorf("no note: %q", tt.Texts())
	}
	shot(t, tt, "bandwidth-alike")
}

// 自动最快 offers to choose by download speed: the choice and how often go
// in the group's policy, and the note says what was chosen and what it
// costs.
func TestChooseByDownload(t *testing.T) {
	u := fixtureUI()
	u.snap.Settings.Paused, u.snap.Settings.SystemProxy = false, true
	tt := ui.NewTester(u.view, 1060, 1000)
	u.home.x, u.home.sub = "routes", "media"
	tt.Frame()
	tt.Advance(2 * time.Second)
	if !tt.HasText("选法") || tt.HasText("每 12 小时") {
		t.Fatalf("by-download row: %q", tt.Texts())
	}
	if err := tt.Click("下载速度"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if q := u.policy("media"); q.DownloadEvery != defaultDownloadEvery || !q.ByDownload() {
		t.Fatalf("policy %+v", q)
	}
	// The download test is a round now, which chooses.
	tt.Frame()
	if !tt.HasText("现在测一轮") || tt.HasText("测下载") {
		t.Errorf("the download button in a group that chooses by it: %q", tt.Texts())
	}
	u.home.bwPlan = bandwidthPlan{Nodes: []string{u.snap.Nodes[0].Name, u.snap.Nodes[5].Name}, MaxBytes: 2 * scheduledCap, Cap: scheduledCap, URL: "https://speed.cloudflare.com/__down?bytes=50000000", Round: true}
	u.home.bwOpen = true
	tt.Frame()
	tt.Advance(time.Second)
	if !slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "现在就测一轮") }) || !tt.HasText("最多会用掉约 40.0 MB 流量（每个节点最多 20.0 MB）。订阅按流量计费的话请留意；有的机场不允许测速。") {
		t.Errorf("the round's dialog: %q", tt.Texts())
	}
	shot(t, tt, "bandwidth-round-dialog")
	u.home.bwOpen = false
	tt.Frame()
	if err := tt.Click("每 6 小时"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if q := u.policy("media"); q.DownloadEvery != 6 {
		t.Fatalf("every %d", q.DownloadEvery)
	}

	now := time.Now()
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "media" {
			u.snap.Routes[i].Download = &DownloadPickView{Node: u.snap.Nodes[1].Name, Mbps: 188, At: now.Add(-2 * time.Hour), Next: now.Add(4 * time.Hour)}
		}
	}
	tt.Frame()
	tt.Advance(time.Second)
	found := slices.ContainsFunc(tt.Texts(), func(s string) bool {
		return strings.Contains(s, "下载最快（188 Mbps）") && strings.Contains(s, "每天 4 轮")
	})
	if !found {
		t.Errorf("no pick note: %q", tt.Texts())
	}
	shot(t, tt, "bandwidth-by-download")

	if err := tt.Click("延迟"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if q := u.policy("media"); q.ByDownload() {
		t.Errorf("back to delay: %+v", q)
	}
}

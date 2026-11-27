package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 日志 › 设备: a device's own route stands beside its name, and its detail
// offers the rules, each route group and leaving it out.
func TestActivityDeviceRoutes(t *testing.T) {
	u := gatewayFixture()
	now := time.Now()
	u.snap.Devices = []DeviceView{
		{IP: "192.168.31.101", MAC: "a4:83:e7:01:02:03", Name: "Living-Room-TV", Via: "auto", Online: true, Seen: now, Events: []DeviceEvent{}, Route: "media", RouteName: "流媒体"},
		{IP: "192.168.31.60", Name: "Camera", Via: "auto", Online: true, Seen: now, Events: []DeviceEvent{}, Bypass: true, Note: "bowel 给它的网关和 DNS 是路由器，它不经过这里（改了之后最晚下次续租生效，10 分钟内；重连 Wi-Fi 立刻生效）"},
		{IP: "192.168.31.40", Via: "manual", Online: true, Seen: now, Events: []DeviceEvent{}},
	}
	u.page = "activity"
	u.act.showDevices()
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	for _, w := range []string{"→ 流媒体", "不经过网关"} {
		if !tt.HasText(w) {
			t.Errorf("no %q in %q", w, tt.Texts())
		}
	}
	if err := tt.Click("Living-Room-TV"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	shot(t, tt, "activity-device-route")
	if !tt.HasText("全部走 流媒体") {
		t.Errorf("the TV's route isn't chosen: %q", tt.Texts())
	}
	if err := tt.Click("Living-Room-TV"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Camera"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("bowel 给它的网关和 DNS 是路由器，它不经过这里（改了之后最晚下次续租生效，10 分钟内；重连 Wi-Fi 立刻生效）") {
		t.Errorf("the camera's note: %q", tt.Texts())
	}
	// A device with no route of its own says what one would do.
	if err := tt.Click("Camera"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("192.168.31.40"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	for _, w := range []string{"按规则分流（和其他设备一样）", "选一个路由组，这台设备的全部流量就都走它；拦截和锁定出口的路由组仍然对它生效。"} {
		if !tt.HasText(w) {
			t.Errorf("no %q in %q", w, tt.Texts())
		}
	}
}

// The choices: the rules first, the groups in the list's order with the
// default one last, leaving it out at the end; under a provider's own
// routing no group of bowel's.
func TestDeviceRouteChoices(t *testing.T) {
	all := []RouteView{{ID: "default", Name: "其他流量"}, {ID: "reject", Name: "拦截"}, {ID: "media", Name: "流媒体"}}
	labels, ids := deviceRouteChoices(all, false)
	want := []string{deviceRules, "reject", "media", "default", deviceBypass}
	if len(ids) != len(want) || len(labels) != len(ids) {
		t.Fatalf("%v %v", labels, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids %v, want %v", ids, want)
		}
	}
	if _, ids := deviceRouteChoices(all, true); len(ids) != 2 {
		t.Fatalf("under a provider's routing: %v", ids)
	}
}

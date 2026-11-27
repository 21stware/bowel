package core

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/routes"
)

// deviceInput is the template with a rule in Block, the dev group locked,
// and three devices: a TV to the streaming group, a camera direct, a
// blocked tablet.
func deviceInput() Input {
	users := []routes.UserRule{{Kind: routes.KindSuffix, Value: "ads.example", Route: routes.Reject}}
	var p routes.Plan
	p.SetLocked("dev", true)
	in := Input{Routes: routes.Build(p, users), UserRules: users, Mode: "rule"}
	in.Devices = []DeviceRoute{{IP: "192.168.1.50", Route: "media"}, {IP: "192.168.1.60"}, {IP: "192.168.1.70", Route: routes.Reject}}
	return in
}

// A device's route takes all its traffic before the groups', with Block
// above it and the locked group repeated for it (Block and AI come first
// in the template; the dev group is below the streaming one); a blocked
// device has nothing above it.
func TestDeviceRules(t *testing.T) {
	_, rules, _ := lockConfig(t, deviceInput())
	at := func(line string) int {
		t.Helper()
		i := slices.Index(rules, line)
		if i < 0 {
			t.Fatalf("no %q in\n%s", line, strings.Join(rules, "\n"))
		}
		return i
	}
	if rules[0] != "SRC-IP-CIDR,192.168.1.70/32,REJECT" {
		t.Errorf("the blocked device isn't first: %v", rules[:3])
	}
	block := at("DOMAIN-SUFFIX,ads.example,REJECT")
	tv := at("SRC-IP-CIDR,192.168.1.50/32,流媒体")
	cam := at("SRC-IP-CIDR,192.168.1.60/32,DIRECT")
	ai := at("DOMAIN-SUFFIX,openai.com,AI 服务")
	media := at("DOMAIN-SUFFIX,netflix.com,流媒体")
	devl := at("DOMAIN-SUFFIX,npmjs.org,开发")
	if !(block < tv && tv < cam && cam < ai && ai < media && media < devl) {
		t.Errorf("order: block %d, tv %d, camera %d, ai %d, media %d, dev %d", block, tv, cam, ai, media, devl)
	}
	// The locked group's rules hold for the devices, before their own, and
	// its UDP is held as it is for everyone.
	for _, ip := range []string{"192.168.1.50", "192.168.1.60"} {
		own := at("SRC-IP-CIDR," + ip + "/32," + map[string]string{"192.168.1.50": "流媒体", "192.168.1.60": "DIRECT"}[ip])
		cp := at("AND,((SRC-IP-CIDR," + ip + "/32),(DOMAIN-SUFFIX,npmjs.org)),开发")
		hold := at("AND,((SRC-IP-CIDR," + ip + "/32),(NETWORK,UDP),(DOMAIN-SUFFIX,npmjs.org)),REJECT")
		if !(cp < hold && hold < own) {
			t.Errorf("%s: copy %d, hold %d, its own %d", ip, cp, hold, own)
		}
	}
	for _, r := range rules {
		if strings.Contains(r, "192.168.1.70") && r != rules[0] {
			t.Errorf("the blocked device has another line: %s", r)
		}
		// Block is at the top already: it isn't repeated.
		if strings.HasPrefix(r, "AND,((SRC-IP-CIDR") && strings.Contains(r, "ads.example") {
			t.Errorf("Block repeated: %s", r)
		}
		if strings.HasPrefix(r, "AND,((SRC-IP-CIDR") && strings.Contains(r, "PROCESS-") {
			t.Errorf("a program's rule repeated for a device: %s", r)
		}
	}
}

// Block moved below a group: its rules are repeated for the devices, and
// what is above it stays above them.
func TestDeviceRulesBlockBelow(t *testing.T) {
	in := deviceInput()
	var p routes.Plan
	p.MoveGroup(routes.Reject, "dev") // below 开发
	in.Routes = routes.Build(p, in.UserRules)
	in.Devices = in.Devices[:1]
	_, rules, _ := lockConfig(t, in)
	tv := slices.Index(rules, "SRC-IP-CIDR,192.168.1.50/32,流媒体")
	cp := slices.Index(rules, "AND,((SRC-IP-CIDR,192.168.1.50/32),(DOMAIN-SUFFIX,ads.example)),REJECT")
	ai := slices.Index(rules, "DOMAIN-SUFFIX,openai.com,AI 服务")
	if tv < 0 || cp < 0 || cp > tv || tv > ai {
		t.Errorf("tv %d, block's copy %d, ai %d\n%s", tv, cp, ai, strings.Join(rules, "\n"))
	}
}

// Device routes are rules: outside rule mode, and with a provider's own
// routing, there are none; nor for a group that is gone.
func TestDeviceRulesOnlyInRuleMode(t *testing.T) {
	for _, mode := range []string{"global", "direct"} {
		in := deviceInput()
		in.Mode = mode
		_, rules, _ := lockConfig(t, in)
		for _, r := range rules {
			if strings.Contains(r, "SRC-IP-CIDR") {
				t.Errorf("%s: %s", mode, r)
			}
		}
	}
	in := deviceInput()
	in.Devices = []DeviceRoute{{IP: "192.168.1.50", Route: "gone"}, {IP: "not an address"}}
	_, rules, _ := lockConfig(t, in)
	for _, r := range rules {
		if strings.Contains(r, "SRC-IP-CIDR") {
			t.Errorf("a device without a group: %s", r)
		}
	}
}

// The real core takes the devices' lines, rule sets and keywords among the
// lines repeated for them.
func TestDeviceRulesAccepted(t *testing.T) {
	bin, geo := bundledMihomo(t)
	dir := coreDir(t, geo)
	users := []routes.UserRule{
		{Kind: routes.KindKeyword, Value: "a(b", Route: "dev"},
		{Kind: routes.KindIP, Value: "203.0.113.0/24", Route: "dev"},
		{Kind: routes.KindGeoIP, Value: "cn", Route: routes.Reject},
	}
	var p routes.Plan
	p.SetLocked("dev", true)
	p.MoveGroup(routes.Reject, "dev")
	id := p.AddRuleSet(users, "dev", "list", "https://x.example/a.yaml")
	set := filepath.Join(dir, "set.yaml")
	if err := os.WriteFile(set, []byte("payload:\n  - DOMAIN-SUFFIX,set.bowel-test.dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := devInput("rule", devnet.Overlay{})
	in.Routes, in.UserRules = routes.Build(p, users), users
	in.RuleSets = []RuleSetRef{{ID: id, Path: set, Behavior: "classical"}}
	in.Devices = []DeviceRoute{{IP: "192.168.1.50", Route: "media"}, {IP: "192.168.1.60"}, {IP: "192.168.1.70", Route: routes.Reject}, {IP: "192.168.1.80", Route: "dev"}}
	b, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"AND,((SRC-IP-CIDR,192.168.1.50/32),(RULE-SET," + routes.RuleSetProvider(id) + ",no-resolve)),开发",
		`AND,((SRC-IP-CIDR,192.168.1.60/32),(DOMAIN-REGEX,a\x28b)),开发`,
		"AND,((SRC-IP-CIDR,192.168.1.50/32),(GEOIP,cn,no-resolve)),REJECT",
		"AND,((NETWORK,UDP),(SRC-IP-CIDR,192.168.1.80/32)),REJECT",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("no %s in\n%s", want, b)
		}
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "-t", "-d", dir, "-f", path).CombinedOutput(); err != nil {
		t.Errorf("%v\n%s\n%s", err, out, b)
	}
}

// Through the real core: what comes from a device's address goes the way
// its route says, whatever the groups' rules would do with it. The test is
// the device, from 127.0.0.1, whose address the group 直连 sends direct.
func TestDeviceRuleRuns(t *testing.T) {
	bin, geo := bundledMihomo(t)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer web.Close()
	// What the device fetches: not the test site, which the groups' tests
	// fetch through the node too.
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer dest.Close()
	node, hits := connectProxy(t)
	host := strings.TrimPrefix(dest.URL, "http://")
	for _, c := range []struct {
		name, route string
		via, ok     bool // through the node; answered
	}{{"none", "-", false, true}, {"media", "media", true, true}, {"direct", "", false, true}, {"block", routes.Reject, false, false}} {
		t.Run(c.name, func(t *testing.T) {
			dir := coreDir(t, geo)
			nodes := fmt.Sprintf("proxies:\n  - {name: hop, type: http, server: 127.0.0.1, port: %d}\n", node)
			if err := os.WriteFile(filepath.Join(dir, "nodes.yaml"), []byte(nodes), 0o600); err != nil {
				t.Fatal(err)
			}
			in := devInput("rule", devnet.Overlay{})
			in.FindProcess, in.TestURL = "off", web.URL+"/"
			in.Providers = []ProviderRef{{ID: "a", Path: "nodes.yaml"}}
			if c.route != "-" {
				in.Devices = []DeviceRoute{{IP: "127.0.0.1", Route: c.route}}
			}
			port, log := runCore(t, bin, dir, in)
			proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
			client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
			before := hits(host)
			answered := false
			// The streaming group's test picks the node first: until then
			// its selector has no node to go by.
			for i := 0; i < 40 && !answered; i++ {
				if res, err := client.Get(dest.URL + "/"); err == nil {
					body, _ := io.ReadAll(res.Body)
					res.Body.Close()
					answered = string(body) == "ok"
				}
				if !answered && c.ok {
					time.Sleep(200 * time.Millisecond)
				}
				if !c.ok && i > 3 {
					break
				}
			}
			via := hits(host) > before
			if answered != c.ok || (c.ok && via != c.via) {
				t.Errorf("answered %v (want %v), by the node %v (want %v)\n%s", answered, c.ok, via, c.via, log.String())
			}
		})
	}
}

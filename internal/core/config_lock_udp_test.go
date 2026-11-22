package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/routes"
)

const holdPrefix = "AND,((NETWORK,UDP),("

// lineParts is a rule line cut into the rule and its target.
func lineParts(line string) (rule, target string) {
	noResolve := strings.HasSuffix(line, ",no-resolve")
	line = strings.TrimSuffix(line, ",no-resolve")
	i := strings.LastIndex(line, ",")
	rule, target = line[:i], line[i+1:]
	if noResolve {
		rule += ",no-resolve"
	}
	return rule, target
}

// lockedWith is the template with the AI group locked and users in it.
func lockedWith(users []routes.UserRule) []routes.Route {
	var p routes.Plan
	p.SetLocked("ai", true)
	return routes.Build(p, users)
}

// TestLockHoldsUDP: each of a locked group's rules is followed by the line
// that refuses the UDP its node can't carry, in every mode; no other
// group's rule is.
func TestLockHoldsUDP(t *testing.T) {
	users := []routes.UserRule{
		{Kind: routes.KindApp, Value: "ChatGPT.exe", Route: "ai"},
		{Kind: routes.KindApp, Value: "Telegram.exe", Route: "media"},
		{Kind: routes.KindIP, Value: "203.0.113.0/24", Route: "ai"},
	}
	for _, mode := range []string{"rule", "global", "direct"} {
		_, rules, _ := lockConfig(t, Input{Routes: lockedWith(users), UserRules: users, Mode: mode})
		i := slices.Index(rules, "PROCESS-NAME,ChatGPT.exe,AI 服务")
		if i < 0 || i+1 == len(rules) || rules[i+1] != "AND,((NETWORK,UDP),(PROCESS-NAME,ChatGPT.exe)),REJECT" {
			t.Errorf("%s: the app's rule isn't followed by its hold:\n%s", mode, strings.Join(rules, "\n"))
		}
		if !has(rules, "AND,((NETWORK,UDP),(IP-CIDR,203.0.113.0/24,no-resolve)),REJECT") {
			t.Errorf("%s: the address rule has no hold", mode)
		}
		locked := 0
		for j, line := range rules {
			if strings.HasPrefix(line, holdPrefix) {
				continue
			}
			rule, target := lineParts(line)
			next := ""
			if j+1 < len(rules) {
				next = rules[j+1]
			}
			want, _ := udpHold(rule)
			switch {
			case target == "AI 服务":
				locked++
				if next != want {
					t.Errorf("%s: %s is followed by %q", mode, line, next)
				}
			case strings.HasPrefix(next, holdPrefix):
				t.Errorf("%s: %s, not locked, is followed by %s", mode, line, next)
			}
		}
		if locked < 3 {
			t.Errorf("%s: %d locked rules", mode, locked)
		}
	}
}

// TestLockGuardHoldsNothing: with takeover off a locked group's rules are
// refused outright, UDP and all, so they need no hold.
func TestLockGuardHoldsNothing(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindApp, Value: "ChatGPT.exe", Route: "ai"}}
	_, rules, _ := lockConfig(t, Input{Routes: lockedWith(users), UserRules: users, Mode: "rule", Guard: true})
	if !has(rules, "PROCESS-NAME,ChatGPT.exe,REJECT") {
		t.Errorf("the app's rule isn't refused: %v", rules)
	}
	for _, r := range rules {
		if strings.HasPrefix(r, holdPrefix) {
			t.Errorf("a hold with takeover off: %s", r)
		}
	}
}

// TestUnlockedHoldsNothing: without a lock nothing changes.
func TestUnlockedHoldsNothing(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindApp, Value: "ChatGPT.exe", Route: "ai"}}
	_, rules, _ := lockConfig(t, Input{Routes: routes.Build(routes.Plan{}, users), UserRules: users, Mode: "rule"})
	for _, r := range rules {
		if strings.HasPrefix(r, holdPrefix) {
			t.Errorf("a hold without a lock: %s", r)
		}
	}
}

// TestLogicSafe: a value with a parenthesis in it would cut the logic rule
// in the wrong place, so it is written as a regular expression that
// matches what the rule does, and nothing else.
func TestLogicSafe(t *testing.T) {
	for _, c := range []struct {
		rule, kind  string
		match, miss []string
	}{
		{"PROCESS-NAME,Foo (x86).exe", "PROCESS-NAME-REGEX", []string{"Foo (x86).exe", "foo (X86).EXE"}, []string{"Foo x86.exe", "Foo (x86)aexe", "AFoo (x86).exe", "Foo (x86).exe2"}},
		{"PROCESS-NAME,微信 (测试).exe", "PROCESS-NAME-REGEX", []string{"微信 (测试).exe"}, []string{"微信 测试.exe"}},
		{"PROCESS-PATH-WILDCARD,*/Foo (2).app/*", "PROCESS-PATH-REGEX", []string{"/Applications/Foo (2).app/Contents/MacOS/foo"}, []string{"/Applications/Foo 2.app/Contents/MacOS/foo", "Foo (2).app/x"}},
		{"PROCESS-PATH-WILDCARD,*/A?(b).app/*", "PROCESS-PATH-REGEX", []string{"/x/Ac(b).app/y"}, []string{"/x/A(b).app/y"}},
		{"DOMAIN-KEYWORD,a(b", "DOMAIN-REGEX", []string{"xa(by.com"}, []string{"xaby.com"}},
		{"DOMAIN,a(b.com", "DOMAIN-REGEX", []string{"a(b.com"}, []string{"xa(b.com", "a(bxcom"}},
	} {
		got, ok := logicSafe(c.rule)
		kind, expr, _ := strings.Cut(got, ",")
		if !ok || kind != c.kind || strings.ContainsAny(expr, "(),") {
			t.Errorf("%s: %q, %v", c.rule, got, ok)
			continue
		}
		re := regexp.MustCompile("(?i)" + expr)
		for _, s := range c.match {
			if !re.MatchString(s) {
				t.Errorf("%s: %s misses %q", c.rule, expr, s)
			}
		}
		for _, s := range c.miss {
			if re.MatchString(s) {
				t.Errorf("%s: %s matches %q", c.rule, expr, s)
			}
		}
	}
	for _, rule := range []string{"DOMAIN-SUFFIX,openai.com", "IP-CIDR,1.2.3.0/24,no-resolve", "RULE-SET,rs-x,no-resolve", "PROCESS-NAME,ChatGPT.exe"} {
		if got, ok := logicSafe(rule); !ok || got != rule {
			t.Errorf("%s became %q, %v", rule, got, ok)
		}
	}
	if _, ok := logicSafe("DOMAIN-SUFFIX,a(b"); ok {
		t.Error("a kind it can't write was written")
	}
}

// The real core takes every hold a locked group can get: an app's, one
// with parentheses in its name, a keyword's, an address's, a rule set's.
func TestLockHoldsAccepted(t *testing.T) {
	bin, geo := bundledMihomo(t)
	dir := coreDir(t, geo)
	users := []routes.UserRule{
		{Kind: routes.KindApp, Value: "ChatGPT.exe", Route: "ai"},
		{Kind: routes.KindApp, Value: "Foo (x86).exe", Route: "ai"},
		{Kind: routes.KindKeyword, Value: "a(b", Route: "ai"},
		{Kind: routes.KindIP, Value: "203.0.113.0/24", Route: "ai"},
	}
	var p routes.Plan
	p.SetLocked("ai", true)
	id := p.AddRuleSet(users, "ai", "list", "https://x.example/a.yaml")
	set := filepath.Join(dir, "set.yaml")
	if err := os.WriteFile(set, []byte("payload:\n  - DOMAIN-SUFFIX,set.bowel-test.dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := devInput("rule", devnet.Overlay{})
	in.Routes, in.UserRules = routes.Build(p, users), users
	in.RuleSets = []RuleSetRef{{ID: id, Path: set, Behavior: "classical"}}
	b, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`AND,((NETWORK,UDP),(PROCESS-NAME-REGEX,^Foo\x20\x28x86\x29\x2eexe$)),REJECT`,
		"AND,((NETWORK,UDP),(RULE-SET," + routes.RuleSetProvider(id) + ",no-resolve)),REJECT",
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

// Through the real core, with a node that carries TCP but not UDP, as an
// http node does: a group's UDP, which the core passes on to the rules
// after, goes out direct; once the group is locked it is refused. Its TCP
// goes by the node either way.
func TestLockUDPRuns(t *testing.T) {
	bin, geo := bundledMihomo(t)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer web.Close()
	node, _ := connectProxy(t)
	echo := udpEcho(t)
	users := []routes.UserRule{{Kind: routes.KindIP, Value: "127.0.0.1/32", Route: "ai"}}
	for _, lock := range []bool{false, true} {
		t.Run(fmt.Sprintf("locked=%v", lock), func(t *testing.T) {
			var p routes.Plan
			p.SetLocked("ai", lock)
			dir := coreDir(t, geo)
			nodes := fmt.Sprintf("proxies:\n  - {name: hop, type: http, server: 127.0.0.1, port: %d}\n", node)
			if err := os.WriteFile(filepath.Join(dir, "nodes.yaml"), []byte(nodes), 0o600); err != nil {
				t.Fatal(err)
			}
			in := devInput("rule", devnet.Overlay{})
			in.Routes, in.UserRules = routes.Build(p, users), users
			in.FindProcess, in.TestURL = "off", web.URL+"/"
			in.Providers = []ProviderRef{{ID: "a", Path: "nodes.yaml"}}
			port, log := runCore(t, bin, dir, in)

			// TCP by the node: also what tells that the group's test has
			// picked the node, not the REJECT a locked group holds.
			proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
			client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
			answered := false
			for i := 0; i < 40 && !answered; i++ {
				if res, err := client.Get(web.URL + "/"); err == nil {
					body, _ := io.ReadAll(res.Body)
					res.Body.Close()
					answered = string(body) == "ok"
				}
				if !answered {
					time.Sleep(200 * time.Millisecond)
				}
			}
			if !answered {
				t.Fatalf("TCP by the node wasn't answered:\n%s", log.String())
			}
			got := ""
			for i := 0; i < 3 && got == ""; i++ {
				got = udpThrough(t, port, echo, "ping", time.Second)
			}
			switch {
			case lock && got != "":
				t.Errorf("a locked group's UDP got out: %q\n%s", got, log.String())
			case !lock && got != "ping":
				t.Errorf("an unlocked group's UDP wasn't passed on (the core changed?): %q\n%s", got, log.String())
			}
		})
	}
}

// runCore runs the bundled core on in's config in dir until the test
// ends, and returns its mixed port once it is open.
func runCore(t *testing.T, bin, dir string, in Input) (int, *lockedBuf) {
	t.Helper()
	port, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	in.MixedPort, in.ProbePort = port, probe
	b, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	log := &lockedBuf{}
	cmd := exec.CommandContext(ctx, bin, "-d", dir, "-f", path)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); cmd.Wait() })
	for i := 0; i < 100; i++ {
		if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond); err == nil {
			c.Close()
			return port, log
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the core did not start:\n%s", log.String())
	return 0, nil
}

// connectProxy is an HTTP proxy that only does CONNECT: a node that
// carries TCP and no UDP. It returns its port, and how many times it was
// asked for each host:port.
func connectProxy(t *testing.T) (int, func(target string) int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	asked := map[string]int{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil || req.Method != http.MethodConnect {
					return
				}
				mu.Lock()
				asked[req.Host]++
				mu.Unlock()
				up, err := net.Dial("tcp", req.Host)
				if err != nil {
					io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
					return
				}
				defer up.Close()
				io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
				go io.Copy(up, br)
				io.Copy(c, up)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func(target string) int {
		mu.Lock()
		defer mu.Unlock()
		return asked[target]
	}
}

// udpEcho sends every datagram back where it came from.
func udpEcho(t *testing.T) *net.UDPAddr {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(buf[:n], from)
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr)
}

// udpThrough sends msg to to by the core's SOCKS5 UDP, and returns what
// comes back within wait, or "".
func udpThrough(t *testing.T, port int, to *net.UDPAddr, msg string, wait time.Duration) string {
	t.Helper()
	ctl, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()
	ctl.SetDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1500)
	if _, err := ctl.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(ctl, buf[:2]); err != nil || buf[1] != 0 {
		t.Fatalf("SOCKS5 greeting: %v %v", err, buf[:2])
	}
	if _, err := ctl.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(ctl, buf[:4]); err != nil || buf[1] != 0 {
		t.Fatalf("UDP ASSOCIATE: %v %v", err, buf[:4])
	}
	addrLen := map[byte]int{1: 4, 4: 16}[buf[3]]
	if buf[3] == 3 {
		if _, err := io.ReadFull(ctl, buf[:1]); err != nil {
			t.Fatal(err)
		}
		addrLen = int(buf[0])
	}
	if _, err := io.ReadFull(ctl, buf[:addrLen+2]); err != nil {
		t.Fatal(err)
	}
	relay := int(binary.BigEndian.Uint16(buf[addrLen : addrLen+2]))
	pc, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: relay})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	pkt := append([]byte{0, 0, 0, 1}, to.IP.To4()...)
	pkt = binary.BigEndian.AppendUint16(pkt, uint16(to.Port))
	if _, err := pc.Write(append(pkt, msg...)); err != nil {
		t.Fatal(err)
	}
	pc.SetReadDeadline(time.Now().Add(wait))
	n, err := pc.Read(buf)
	if err != nil || n < 4 {
		return ""
	}
	head := 4 + map[byte]int{1: 4 + 2, 4: 16 + 2}[buf[3]]
	if buf[3] == 3 {
		head = 5 + int(buf[4]) + 2
	}
	if n < head {
		return ""
	}
	return string(buf[head:n])
}

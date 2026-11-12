package sysproxy

import (
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestParseWinServer(t *testing.T) {
	for _, c := range []struct {
		in                 string
		web, secure, socks Endpoint
	}{
		{"", Endpoint{}, Endpoint{}, Endpoint{}},
		{"127.0.0.1:7899", Endpoint{Host: "127.0.0.1", Port: 7899}, Endpoint{Host: "127.0.0.1", Port: 7899}, Endpoint{}},
		{"http=proxy.corp:8080;https=proxy.corp:8443;socks=127.0.0.1:1080",
			Endpoint{Host: "proxy.corp", Port: 8080}, Endpoint{Host: "proxy.corp", Port: 8443}, Endpoint{Host: "127.0.0.1", Port: 1080}},
		{"http://127.0.0.1:7890", Endpoint{Host: "127.0.0.1", Port: 7890}, Endpoint{Host: "127.0.0.1", Port: 7890}, Endpoint{}},
		{"HTTP=a:1; socks=b", Endpoint{Host: "a", Port: 1}, Endpoint{}, Endpoint{Host: "b", Port: 1080}},
		{"[::1]:7899", Endpoint{Host: "::1", Port: 7899}, Endpoint{Host: "::1", Port: 7899}, Endpoint{}},
		{"proxy.corp", Endpoint{Host: "proxy.corp", Port: 80}, Endpoint{Host: "proxy.corp", Port: 80}, Endpoint{}},
		{"https=s:443;d:3128", Endpoint{Host: "d", Port: 3128}, Endpoint{Host: "s", Port: 443}, Endpoint{}},
	} {
		web, secure, socks := parseWinServer(c.in)
		if web != c.web || secure != c.secure || socks != c.socks {
			t.Errorf("parseWinServer(%q) = %+v %+v %+v, want %+v %+v %+v", c.in, web, secure, socks, c.web, c.secure, c.socks)
		}
	}
}

func TestWinServer(t *testing.T) {
	e := func(h string, p int) Endpoint { return Endpoint{Host: h, Port: p} }
	for _, c := range []struct {
		web, secure, socks Endpoint
		want               string
	}{
		{e("127.0.0.1", 7899), e("127.0.0.1", 7899), Endpoint{}, "127.0.0.1:7899"},
		{e("::1", 7899), e("::1", 7899), Endpoint{}, "[::1]:7899"},
		{e("a", 1), e("b", 2), e("c", 3), "http=a:1;https=b:2;socks=c:3"},
		{e("a", 1), e("a", 1), e("c", 3), "http=a:1;https=a:1;socks=c:3"},
		{Endpoint{}, Endpoint{}, Endpoint{}, ""},
	} {
		if got := winServer(c.web, c.secure, c.socks); got != c.want {
			t.Errorf("winServer = %q, want %q", got, c.want)
		}
	}
}

func TestWinBypass(t *testing.T) {
	got, _ := winBypass([]string{"localhost", "127.0.0.1", "*.local", "169.254/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fd00::/8", "timestamp.apple.com", "10.1.2.3/32"})
	want := "localhost;127.0.0.1;*.local;169.254.*;10.*;" +
		"172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;" +
		"172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;" +
		"192.168.*;timestamp.apple.com;10.1.2.3;<local>"
	if got != want {
		t.Errorf("winBypass =\n%s\nwant\n%s", got, want)
	}
	if got, dropped := winBypass([]string{"<local>", "10.0.0.0/8", "10.0.0.0/8"}); got != "10.*;<local>" || dropped != nil {
		t.Errorf("winBypass with duplicates = %q", got)
	}
	if got := wildcards("10.0.0.0/30"); !slices.Equal(got, []string{"10.0.0.0", "10.0.0.1", "10.0.0.2", "10.0.0.3"}) {
		t.Errorf("wildcards /30 = %v", got)
	}
	if got := wildcards("0.0.0.0/0"); !slices.Equal(got, []string{"*"}) {
		t.Errorf("wildcards /0 = %v", got)
	}
}

// An IPv6 address in ProxyOverride makes InternetSetOption reject the whole
// call, so none may reach it; everything else is written as before.
func TestWinBypassIPv6(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []string
		want string
	}{
		{"loopback", []string{"::1"}, "<local>"},
		{"bracketed", []string{"[::1]"}, "<local>"},
		{"bracketed with port", []string{"[::1]:8080"}, "<local>"},
		{"full address", []string{"2001:db8::1"}, "<local>"},
		{"wildcard", []string{"fe80::*"}, "<local>"},
		{"surrounded by spaces", []string{" ::1 "}, "<local>"},
		{"network", []string{"fd00::/8"}, "<local>"},
		{"host and port kept", []string{"example.com:8080"}, "example.com:8080;<local>"},
		{"IPv4 and port kept", []string{"10.0.0.1:8080"}, "10.0.0.1:8080;<local>"},
		{"IPv4 kept", []string{"127.0.0.1"}, "127.0.0.1;<local>"},
		{"IPv4 wildcard kept", []string{"192.168.*"}, "192.168.*;<local>"},
		{"name wildcard kept", []string{"*.local"}, "*.local;<local>"},
		{"the devnet loopback literals", []string{"localhost", "127.0.0.1", "::1"}, "localhost;127.0.0.1;<local>"},
	} {
		if got, _ := winBypass(c.in); got != c.want {
			t.Errorf("%s: winBypass(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestWinServiceRoundTrip(t *testing.T) {
	for _, ws := range []winSettings{
		{flags: winDirect, server: "", override: "", pac: ""},
		{flags: winDirect | winProxy, server: "proxy.corp:8080", override: "*.corp;<local>"},
		{flags: winDirect | winPAC | winAutoDetect, server: "proxy.corp:8080", override: "<local>", pac: "http://wpad/proxy.pac"},
		{flags: winDirect | winProxy, server: "http=a:1;https=b:2;socks=c:3"},
		{flags: winDirect, server: "a:1", pac: "http://kept-while-off/p.pac"}, // off, but kept
	} {
		sv := winService(ws)
		if got := sv.winSettings(); got != ws {
			t.Errorf("round trip of %+v gave %+v (via %+v)", ws, got, sv)
		}
	}
}

func TestWinServiceForget(t *testing.T) {
	// A snapshot taken while Moho held the proxy restores to none, and
	// keeps the PAC script the user had.
	saved := Saved{Services: []Service{winService(winSettings{
		flags: winDirect | winProxy, server: "127.0.0.1:7899", override: "localhost;<local>", pac: "http://wpad/p.pac",
	})}}
	saved.Forget(7899)
	got := saved.Services[0].winSettings()
	want := winSettings{flags: winDirect, override: strings.Join(DefaultBypass, ";"), pac: "http://wpad/p.pac"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestWinServiceTurnOffDead(t *testing.T) {
	// The address and bypass list stay; only the switch goes off.
	saved := Saved{Services: []Service{winService(winSettings{flags: winDirect | winProxy, server: "127.0.0.1:7890", override: "localhost;<local>"})}}
	saved.TurnOffDead(func(string, int) bool { return false })
	got := saved.Services[0].winSettings()
	want := winSettings{flags: winDirect, server: "127.0.0.1:7890", override: "localhost;<local>"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestWinStatus(t *testing.T) {
	ours := winSettings{flags: winDirect | winProxy, server: "127.0.0.1:7899"}
	if s := winStatus(ours, 7899); !s.On || !s.Ours || s.Host != "127.0.0.1" || s.Port != 7899 {
		t.Errorf("ours: %+v", s)
	}
	if s := winStatus(ours, 7890); !s.On || s.Ours {
		t.Errorf("another port: %+v", s)
	}
	off := ours
	off.flags = winDirect
	if s := winStatus(off, 7899); s.On || s.Ours {
		t.Errorf("switched off: %+v", s)
	}
	pac := ours
	pac.flags |= winPAC
	pac.pac = "http://wpad/p.pac"
	if s := winStatus(pac, 7899); !s.On || s.Ours {
		t.Errorf("behind a PAC script: %+v", s)
	}
	socksOnly := winSettings{flags: winDirect | winProxy, server: "socks=127.0.0.1:7899"}
	if s := winStatus(socksOnly, 7899); !s.On || s.Ours {
		t.Errorf("SOCKS only: %+v", s)
	}
}

// What WinINet is not known to take stays out of ProxyOverride, and is
// named with the reason; the rest keeps its order.
func TestWinBypassSanitises(t *testing.T) {
	in := []string{
		"localhost", "", "  ", "*.lan", "::1", "[fe80::1%eth0]", "::ffff:10.0.0.1", "fd7a:115c:a1e0::/48",
		"a;b", "has space", "tab\there", "comma,sep", "ctl\x01", "10.0.0.0/33", "10.0.0.0/8/9", "name/8",
		"host:port", "host:99999", "example.com:8080", "*:8443", ":80",
		"https://intra.corp", "https://intra.corp/", "http://intra.corp:8080", "http://intra.corp/path", "http://[::1]:80",
		"<-loopback>", "<lo;cal>", "*.lan", "100.64.0.0/10", "192.168.1.0/24", "内网.corp",
	}
	got, dropped := winBypass(in)
	var mesh []string
	for i := 64; i < 128; i++ {
		mesh = append(mesh, "100."+strconv.Itoa(i)+".*")
	}
	want := "localhost;*.lan;example.com:8080;*:8443;https://intra.corp;http://intra.corp:8080;<-loopback>;" +
		strings.Join(mesh, ";") + ";192.168.1.*;内网.corp;<local>"
	if got != want {
		t.Errorf("winBypass =\n%s\nwant\n%s", got, want)
	}
	wantDropped := []Dropped{
		{"::1", "ipv6"}, {"[fe80::1%eth0]", "ipv6"}, {"::ffff:10.0.0.1", "ipv6"}, {"fd7a:115c:a1e0::/48", "ipv6"},
		{"a;b", "malformed"}, {"has space", "malformed"}, {"tab\there", "malformed"}, {"comma,sep", "malformed"}, {"ctl\x01", "malformed"},
		{"10.0.0.0/33", "malformed"}, {"10.0.0.0/8/9", "malformed"}, {"name/8", "malformed"},
		{"host:port", "malformed"}, {"host:99999", "malformed"}, {":80", "malformed"},
		{"http://intra.corp/path", "malformed"}, {"http://[::1]:80", "ipv6"}, {"<lo;cal>", "malformed"},
	}
	if !reflect.DeepEqual(dropped, wantDropped) {
		t.Errorf("dropped =\n%v\nwant\n%v", dropped, wantDropped)
	}
	for _, e := range strings.Split(got, ";") {
		if e == "" || strings.ContainsAny(e, " \t,") || strings.Count(e, ":") > 2 {
			t.Errorf("ProxyOverride holds %q", e)
		}
	}
}

// A list that would pass winBypassMax loses what does not fit, whole
// entries at a time and the later ones first; shorter ones after still go in.
func TestWinBypassLength(t *testing.T) {
	var in []string
	for i := range 400 {
		in = append(in, "machine-"+strconv.Itoa(i)+".tailnet.example") // 400 × some 28 units
	}
	in = append(in, "10.128.0.0/9", "z.io") // 128 wildcards that no longer fit, and one name that does
	got, dropped := winBypass(in)
	if n := utf16Len(got); n > winBypassMax {
		t.Fatalf("ProxyOverride is %d long, over %d", n, winBypassMax)
	}
	if !strings.HasPrefix(got, "machine-0.tailnet.example;machine-1.tailnet.example;") || !strings.HasSuffix(got, ";z.io;<local>") {
		t.Errorf("the first entries and the end are not kept: %.60s … %s", got, got[len(got)-40:])
	}
	if strings.Contains(got, "10.1") {
		t.Error("a network went in by halves")
	}
	if len(dropped) == 0 || !slices.Contains(dropped, Dropped{"10.128.0.0/9", "length"}) {
		t.Fatalf("dropped = %v", dropped)
	}
	kept := strings.Count(got, "machine-")
	if kept+len(dropped)-1 != 400 || kept < 60 {
		t.Errorf("%d kept and %d dropped of 400", kept, len(dropped)-1)
	}
	for _, d := range dropped {
		if d.Why != "length" {
			t.Errorf("dropped %v", d)
		}
	}
	// Units, not bytes: a name outside ASCII counts as Windows counts it.
	if utf16Len("内网.corp") != 7 || utf16Len("😀") != 2 {
		t.Error("utf16Len")
	}
	// bowel's own defaults with a tailnet's networks fit whole.
	got, dropped = winBypass([]string{"localhost", "127.0.0.1", "*.local", "169.254/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "*.ts.net"})
	if dropped != nil || !strings.HasSuffix(got, "*.ts.net;<local>") {
		t.Errorf("the defaults lost %v", dropped)
	}
}

// winWrites is a Windows that refuses the writes refuse says it does.
type winWrites struct {
	refuse func(ws winSettings, set winSet) bool
	calls  []string
	has    winSettings
}

func (w *winWrites) write(ws winSettings, set winSet) error {
	call := ""
	if set&setBypass != 0 {
		call = ws.override
	}
	if set&setPAC != 0 {
		call += " +pac"
	}
	w.calls = append(w.calls, call)
	if w.refuse != nil && w.refuse(ws, set) {
		return errors.New("InternetSetOption 75: The parameter is incorrect.")
	}
	if set&setFlags != 0 {
		w.has.flags = ws.flags
	}
	if set&setServer != 0 {
		w.has.server = ws.server
	}
	if set&setBypass != 0 {
		w.has.override = ws.override
	}
	return nil
}

func TestWinTakeover(t *testing.T) {
	const server = "127.0.0.1:7899"
	bypass := []string{"localhost", "::1", "*.corp", "10.0.0.0/8"}
	full, _ := winBypass(bypass)
	before := winSettings{flags: winDirect | winPAC, override: "*.theirs", pac: "http://wpad/p.pac"}

	t.Run("taken whole", func(t *testing.T) {
		w := &winWrites{has: before}
		got, err := winTakeover(w.write, server, bypass)
		if err != nil || got.Refused != nil || got.Bypass != full || len(w.calls) != 1 {
			t.Fatalf("got %+v, %v after %q", got, err, w.calls)
		}
		if !reflect.DeepEqual(got.Dropped, []Dropped{{"::1", "ipv6"}}) {
			t.Errorf("dropped = %v", got.Dropped)
		}
		// The script's address is not written: it stays for Windows' page.
		if want := (winSettings{flags: winDirect | winProxy, server: server, override: full, pac: before.pac}); w.has != want {
			t.Errorf("Windows has %+v, want %+v", w.has, want)
		}
		if got.Note() != "" {
			t.Errorf("note = %q", got.Note())
		}
	})

	t.Run("bypass list refused", func(t *testing.T) {
		w := &winWrites{has: before, refuse: func(ws winSettings, set winSet) bool { return strings.Contains(ws.override, "*.corp") }}
		got, err := winTakeover(w.write, server, bypass)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(w.calls, []string{full, winMinimalBypass}) {
			t.Errorf("writes = %q", w.calls)
		}
		want := &Refused{Value: "ProxyOverride", Content: full, Error: "InternetSetOption 75: The parameter is incorrect.", Instead: "minimal"}
		if got.Bypass != winMinimalBypass || !reflect.DeepEqual(got.Refused, want) {
			t.Errorf("got %+v, refused %+v", got, got.Refused)
		}
		if w.has.override != winMinimalBypass || w.has.server != server || w.has.flags != winDirect|winProxy {
			t.Errorf("Windows has %+v", w.has)
		}
		if got.Note() == "" || !strings.Contains(got.Line(), full) || !strings.Contains(got.Line(), "ProxyOverride") {
			t.Errorf("note %q, line %q", got.Note(), got.Line())
		}
	})

	t.Run("any bypass list refused", func(t *testing.T) {
		w := &winWrites{has: before, refuse: func(_ winSettings, set winSet) bool { return set&setBypass != 0 }}
		got, err := winTakeover(w.write, server, bypass)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(w.calls, []string{full, winMinimalBypass, ""}) {
			t.Errorf("writes = %q", w.calls)
		}
		if got.Bypass != "" || got.Refused == nil || got.Refused.Value != "ProxyOverride" || got.Refused.Content != full || got.Refused.Instead != "kept" {
			t.Errorf("got %+v, refused %+v", got, got.Refused)
		}
		if w.has.override != "*.theirs" || w.has.server != server {
			t.Errorf("Windows has %+v", w.has)
		}
		if got.Note() == "" {
			t.Error("no note")
		}
	})

	t.Run("refused whatever the list", func(t *testing.T) {
		w := &winWrites{has: before, refuse: func(winSettings, winSet) bool { return true }}
		got, err := winTakeover(w.write, server, bypass)
		if err == nil || len(w.calls) != 3 {
			t.Fatalf("err = %v after %q", err, w.calls)
		}
		// The error names the value, and still what Windows said.
		for _, part := range []string{`ProxyServer="127.0.0.1:7899"`, "0x3", "InternetSetOption 75: The parameter is incorrect."} {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("error %q lacks %q", err, part)
			}
		}
		if got.Refused == nil || got.Refused.Value != "ProxyServer" || got.Refused.Content != server || got.Refused.Instead != "" || got.Bypass != "" {
			t.Errorf("got %+v, refused %+v", got, got.Refused)
		}
		if w.has != before {
			t.Errorf("Windows has %+v", w.has)
		}
		if got.Note() != "" {
			t.Errorf("a failed takeover has the note %q", got.Note())
		}
	})

	t.Run("the minimal list is not tried twice", func(t *testing.T) {
		w := &winWrites{refuse: func(_ winSettings, set winSet) bool { return set&setBypass != 0 }}
		if _, err := winTakeover(w.write, server, []string{"localhost", "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(w.calls, []string{winMinimalBypass, ""}) {
			t.Errorf("writes = %q", w.calls)
		}
	})
}

func TestWinMinimalBypass(t *testing.T) {
	if !strings.HasPrefix(winMinimalBypass, "localhost;127.*;10.*;172.16.*;") || !strings.HasSuffix(winMinimalBypass, ";172.31.*;192.168.*;<local>") {
		t.Errorf("winMinimalBypass = %q", winMinimalBypass)
	}
}

func TestWinRefusalAndRestoreSet(t *testing.T) {
	ws := winSettings{flags: winDirect | winPAC, server: "proxy.corp:8080", override: "*.corp;<local>", pac: "http://wpad/p.pac"}
	err := winRefusal(ws, setFlags|setServer|setBypass|setPAC, errors.New("InternetSetOption 75: The parameter is incorrect."))
	for _, part := range []string{"flags=0x5", `ProxyServer="proxy.corp:8080"`, `ProxyOverride="*.corp;<local>"`, `AutoConfigURL="http://wpad/p.pac"`, "InternetSetOption 75"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q lacks %q", err, part)
		}
	}
	if err := winRefusal(ws, setFlags, errors.New("x")); strings.Contains(err.Error(), "ProxyServer") {
		t.Errorf("error %q names what was not written", err)
	}
	// The script's address is written back only where it changed.
	if winRestoreSet(ws, winSettings{pac: ws.pac})&setPAC != 0 {
		t.Error("an unchanged script address is written")
	}
	if winRestoreSet(ws, winSettings{})&setPAC == 0 || winRestoreSet(winSettings{}, ws)&setPAC == 0 {
		t.Error("a changed script address is not written")
	}
	if winRestoreSet(winSettings{}, winSettings{}) != setFlags|setServer|setBypass {
		t.Error("restoring without a script writes more than the three")
	}
}

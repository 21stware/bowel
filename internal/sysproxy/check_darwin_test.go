package sysproxy

import "testing"

const scutilOff = `<dictionary> {
  HTTPEnable : 0
  HTTPSEnable : 0
  ProxyAutoConfigEnable : 0
  SOCKSEnable : 0
}
`

const scutilOurs = `<dictionary> {
  ExceptionsList : <array> {
    0 : 127.0.0.1
    1 : localhost
    2 : *.local
  }
  FTPPassive : 1
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
  HTTPSPort : 7890
  HTTPSProxy : 127.0.0.1
  ProxyAutoConfigEnable : 0
  SOCKSEnable : 1
  SOCKSPort : 7890
  SOCKSProxy : 127.0.0.1
}
`

const scutilElsewhere = `<dictionary> {
  HTTPEnable : 1
  HTTPPort : 8080
  HTTPProxy : proxy.corp.example
  HTTPSEnable : 1
  HTTPSPort : 8080
  HTTPSProxy : proxy.corp.example
  SOCKSEnable : 0
}
`

const scutilSocksOnly = `<dictionary> {
  HTTPEnable : 0
  HTTPSEnable : 0
  SOCKSEnable : 1
  SOCKSPort : 1080
  SOCKSProxy : localhost
}
`

func TestParseScutil(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		port int
		want Status
	}{
		{"off", scutilOff, 7890, Status{}},
		{"ours", scutilOurs, 7890, Status{On: true, Host: "127.0.0.1", Port: 7890, Ours: true}},
		{"ours on another port", scutilOurs, 7899, Status{On: true, Host: "127.0.0.1", Port: 7890}},
		{"elsewhere", scutilElsewhere, 8080, Status{On: true, Host: "proxy.corp.example", Port: 8080}},
		{"socks only", scutilSocksOnly, 1080, Status{On: true, Host: "localhost", Port: 1080}},
		{"empty", "", 7890, Status{}},
	} {
		if got := parseScutil(tc.out, tc.port); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	s, err := Check(7890)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("system proxy now: %+v", s)
}

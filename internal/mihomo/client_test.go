package mihomo

import (
	"slices"
	"testing"
	"time"
)

// A Tailscale node leads into a tailnet: a site that doesn't answer
// through it says nothing of it, so only its successes count. When it was
// last tested is still all of its tests'.
func TestPrivateNodeCountsOnlyItsSuccesses(t *testing.T) {
	at := time.Now()
	tests := []History{{Time: at, Delay: 0}, {Time: at.Add(time.Second), Delay: 80}, {Time: at.Add(2 * time.Second), Delay: 0}}
	tailnet := Proxy{Type: "Tailscale", History: tests, Extra: map[string]URLTests{
		"https://a": {History: tests},
		"https://b": {History: []History{{Time: at, Delay: 0}}},
	}}
	if !tailnet.Private() {
		t.Fatal("a Tailscale node is private")
	}
	if got := tailnet.ResultsOf("https://a"); len(got) != 1 || got[0].Delay != 80 {
		t.Errorf("ResultsOf a = %+v, want its one success", got)
	}
	if got := tailnet.ResultsOf("https://b"); len(got) != 0 {
		t.Errorf("ResultsOf b = %+v, want none: it only failed", got)
	}
	if got := tailnet.Results(); len(got) != 1 || got[0].Delay != 80 {
		t.Errorf("Results = %+v", got)
	}
	if got := tailnet.HistoryOf("https://a"); len(got) != 3 {
		t.Errorf("HistoryOf keeps every test: %+v", got)
	}

	proxy := Proxy{Type: "Vless", History: tests, Extra: map[string]URLTests{"https://a": {History: tests}}}
	if proxy.Private() || !slices.Equal(proxy.ResultsOf("https://a"), tests) || !slices.Equal(proxy.Results(), tests) {
		t.Errorf("a proxy's every test counts: %+v", proxy.ResultsOf("https://a"))
	}
}

package bridge

import (
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
)

// TestExpiredPlanIsSaidFirst: as on the desktop, an ended plan is said
// before the broken group it causes, with its website to renew on.
func TestExpiredPlanIsSaidFirst(t *testing.T) {
	p := providerData{ID: "p", Name: "机场 A", Enabled: true, Warn: provider.Expired,
		usage: provider.Usage{Expire: time.Now().Add(-time.Hour).Unix()}, site: "https://renew.example.com"}
	v := viewData{Nodes: []nodeData{{Name: "a"}}, Routes: []routeData{{ID: routes.Default, Name: "默认", Broken: "scope"}}, Providers: []providerData{p}}
	if is := judge("rule", v, nil); is == nil || !strings.Contains(is.Text, "机场 A") || is.Action != "site:https://renew.example.com" {
		t.Fatalf("expired: %+v", is)
	}
	p.Warn, p.usage, p.site = provider.Expire, provider.Usage{Expire: time.Now().Add(72 * time.Hour).Unix()}, ""
	v.Providers, v.Routes = []providerData{p}, nil
	if is := judge("rule", v, nil); is == nil || !strings.Contains(is.Text, "3") || is.Action != "providers" {
		t.Fatalf("ending soon, no website: %+v", is)
	}
}

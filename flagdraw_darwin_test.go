package main

import "testing"

func TestFlagDrawn(t *testing.T) {
	for _, f := range []string{"🇭🇰", "🇯🇵", "🇸🇬", "🇺🇸", "🇨🇭"} {
		if !flagDrawn(f) {
			t.Errorf("flagDrawn(%s) = false, want true", f)
		}
	}
	t.Logf("flagDrawn(🇹🇼) = %v on this Mac", flagDrawn("🇹🇼"))
}

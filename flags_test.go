package main

import (
	"os"
	"regexp"
	"testing"
)

// TestEveryCountryHasAFlag keeps assets/flags in step with the countries
// internal/geo knows.
func TestEveryCountryHasAFlag(t *testing.T) {
	src, err := os.ReadFile("internal/geo/data.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\{"([A-Z]{2})",`).FindAllSubmatch(src, -1) {
		cc := string(m[1])
		if seen[cc] {
			continue
		}
		seen[cc] = true
		if flagSVG(cc) == nil {
			t.Errorf("no assets/flags/%s.svg", cc)
		}
	}
	if len(seen) < 50 {
		t.Fatalf("found only %d countries in data.go", len(seen))
	}
}

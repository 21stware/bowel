package geo

import "testing"

// Providers put characters that take no room inside a name. People read
// the name without them.
func TestVisible(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"香港 01", "香港 01"},
		{"H\u200bK\u200bG 01", "HKG 01"},
		{"\ufeff日\u2060本\u00ad 02", "日本 02"},
		{"a\u180eb\u2061c\u2064d", "abcd"},
		// The joiners and the direction marks are part of the writing.
		{"می\u200cخواهم", "می\u200cخواهم"},
		{"👨\u200d👩\u200d👧 Family", "👨\u200d👩\u200d👧 Family"},
		{"\u200eabc\u200f", "\u200eabc\u200f"},
		// Nothing would be left.
		{"\u200b\u200b", "\u200b\u200b"},
		{"", ""},
	} {
		if got := Visible(tt.in); got != tt.want {
			t.Errorf("Visible(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A place is found in a name whose letters are kept apart by characters
// that are not seen.
func TestFromNameHidden(t *testing.T) {
	for _, tt := range []struct{ name, country, city string }{
		{"H\u200bK\u200b 01", "HK", "香港"},
		{"日\u200b本 03", "JP", "日本"},
		{"To\u2060kyo\ufeff 02", "JP", "东京"},
		{"U\u200cS\u200d Los\u200e Angeles", "US", "洛杉矶"},
		{"新\u00ad加\u200b坡", "SG", "新加坡"},
	} {
		p, ok := FromName(tt.name)
		if !ok || p.Country != tt.country || p.City != tt.city {
			t.Errorf("FromName(%q) = %+v, %v; want %s %s", tt.name, p, ok, tt.country, tt.city)
		}
	}
}

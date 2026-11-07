package geo

import (
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// Display is a place's name, as Place.City holds it, in the interface's
// language. Place.City stays in Chinese, as caches keep it.
// Japanese without a name of its own shows the English one.
func Display(name string) string {
	switch i18n.Lang() {
	case "zh-Hant":
		if s, ok := hantNames[name]; ok {
			return s
		}
	case "ja":
		if s, ok := jaNames[name]; ok {
			return s
		}
		fallthrough
	case "en":
		if en, ok := english[name]; ok {
			return en
		}
	}
	return name
}

// englishOver names the places whose first English alias is not the name
// to show.
var englishOver = map[string]string{"KR": "South Korea", "CZ": "Czechia"}

// english maps the tables' Chinese names to English ones: a row's first
// alias in Latin letters that is not a code.
var english = func() map[string]string {
	m := map[string]string{}
	first := func(aliases []string) string {
		for _, a := range aliases {
			if a != strings.ToUpper(a) && isLatin(a) {
				return a
			}
		}
		return ""
	}
	for _, r := range countryRows {
		m[r.name] = first(r.aliases)
		if en, ok := englishOver[r.code]; ok {
			m[r.name] = en
		}
	}
	for _, r := range cityRows {
		if _, ok := m[r.name]; !ok { // a city named as its country is named so
			m[r.name] = first(r.aliases)
		}
	}
	return m
}()

func isLatin(s string) bool {
	for _, r := range s {
		if r > 0x24F {
			return false
		}
	}
	return true
}

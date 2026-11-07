package geo

import (
	"testing"

	"github.com/oboo/moho/internal/i18n"
)

func TestEnglishNames(t *testing.T) {
	for zh, en := range english {
		if en == "" {
			t.Errorf("%s has no English name", zh)
		}
	}
	defer i18n.Set("zh")
	for zh, want := range map[string]string{"东京": "Tokyo", "韩国": "South Korea", "香港": "Hong Kong", "圣地亚哥": "Santiago", "胡志明": "Ho Chi Minh"} {
		if got := Display(zh); got != zh {
			t.Errorf("Chinese Display(%s) = %s", zh, got)
		}
		i18n.Set("en")
		if got := Display(zh); got != want {
			t.Errorf("Display(%s) = %s, want %s", zh, got, want)
		}
		i18n.Set("zh")
	}
}

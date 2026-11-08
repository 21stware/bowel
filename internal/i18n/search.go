package i18n

// The strings of the settings window's search, kept apart from the main
// tables so that work on other parts merges cleanly.

func init() {
	for k, v := range jaSearch {
		jaTable[k] = v
	}
	for k, v := range hantSearch {
		hantTable[k] = v
	}
}

var jaSearch = map[string]string{
	"搜索":      "検索",
	"搜索设置":    "設定を検索",
	"没有相符的设置": "一致する設定はありません",
	"打开这一页":   "このページを開く",
}

var hantSearch = map[string]string{
	"搜索":      "搜尋",
	"搜索设置":    "搜尋設定",
	"没有相符的设置": "沒有相符的設定",
	"打开这一页":   "打開這一頁",
}

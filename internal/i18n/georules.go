package i18n

// The strings of rules for GeoSite and GeoIP lists, and of rules pasted as
// mihomo writes them, kept apart from the main tables so that work on
// other parts merges cleanly.

func init() {
	for k, v := range jaGeoRules {
		jaTable[k] = v
	}
	for k, v := range hantGeoRules {
		hantTable[k] = v
	}
}

var jaGeoRules = map[string]string{
	"GeoSite 域名库":        "GeoSite リスト",
	"GeoIP 地址库":          "GeoIP リスト",
	"GeoSite 列表 %s 里的域名": "GeoSite リスト %s のドメイン",
	"GeoIP 列表 %s 里的地址":   "GeoIP リスト %s のアドレス",
	"先写列表的名字":            "まずリストの名前を入力してください",
	"%s 规则不能在这里添加，可以写进规则集文件，再导入规则集": "%s ルールはここでは追加できません。ルールセットのファイルに書いて、ルールセットとしてインポートしてください",
	"不是 %s 列表的名字: %s":             "%s リストの名前ではありません: %s",
	"%s 数据里没有叫 %s 的列表，是不是：%s":     "%s データに %s というリストはありません。もしかして：%s",
	"%s 数据里没有叫 %s 的列表":            "%s データに %s というリストはありません",
	"youtube、netflix 或 google@cn": "youtube、netflix、google@cn など",
	"CN、private 或 telegram":       "CN、private、telegram など",
	"数据里没有这个列表，未生效":               "データにこのリストがないため無効です",
	"粘贴链接也可以，会自动提取域名；mihomo 的规则写法也认，如 GEOSITE,youtube。": "リンクを貼り付けるとドメインを自動で取り出します。GEOSITE,youtube のような mihomo のルールの書き方も使えます。",
	"写 GeoSite 列表的名字，列表里的域名都走这个路由组。":                    "GeoSite リストの名前を入力します。リスト内のドメインはこのルートグループを通ります。",
	"写 GeoIP 列表的名字，列表里的地址都走这个路由组。":                      "GeoIP リストの名前を入力します。リスト内のアドレスはこのルートグループを通ります。",
	"写 GeoSite 列表的名字，列表里的域名都走这个路由组。列表来自「设置 › GeoData」。": "GeoSite リストの名前を入力します。リスト内のドメインはこのルートグループを通ります。リストは「設定 › GeoData」のものです。",
	"写 GeoIP 列表的名字，列表里的地址都走这个路由组。列表来自「设置 › GeoData」。":   "GeoIP リストの名前を入力します。リスト内のアドレスはこのルートグループを通ります。リストは「設定 › GeoData」のものです。",
}

var hantGeoRules = map[string]string{
	"GeoSite 域名库":        "GeoSite 網域庫",
	"GeoIP 地址库":          "GeoIP 位址庫",
	"GeoSite 列表 %s 里的域名": "GeoSite 清單 %s 裡的網域",
	"GeoIP 列表 %s 里的地址":   "GeoIP 清單 %s 裡的位址",
	"先写列表的名字":            "先寫清單的名稱",
	"%s 规则不能在这里添加，可以写进规则集文件，再导入规则集": "%s 規則不能在這裡新增，可以寫進規則集檔案，再匯入規則集",
	"不是 %s 列表的名字: %s":             "不是 %s 清單的名稱: %s",
	"%s 数据里没有叫 %s 的列表，是不是：%s":     "%s 資料裡沒有叫 %s 的清單，是不是：%s",
	"%s 数据里没有叫 %s 的列表":            "%s 資料裡沒有叫 %s 的清單",
	"youtube、netflix 或 google@cn": "youtube、netflix 或 google@cn",
	"CN、private 或 telegram":       "CN、private 或 telegram",
	"数据里没有这个列表，未生效":               "資料裡沒有這個清單，未生效",
	"粘贴链接也可以，会自动提取域名；mihomo 的规则写法也认，如 GEOSITE,youtube。": "貼上連結也可以，會自動擷取網域；mihomo 的規則寫法也認得，如 GEOSITE,youtube。",
	"写 GeoSite 列表的名字，列表里的域名都走这个路由组。":                    "寫 GeoSite 清單的名稱，清單裡的網域都走這個路由組。",
	"写 GeoIP 列表的名字，列表里的地址都走这个路由组。":                      "寫 GeoIP 清單的名稱，清單裡的位址都走這個路由組。",
	"写 GeoSite 列表的名字，列表里的域名都走这个路由组。列表来自「设置 › GeoData」。": "寫 GeoSite 清單的名稱，清單裡的網域都走這個路由組。清單來自「設定 › GeoData」。",
	"写 GeoIP 列表的名字，列表里的地址都走这个路由组。列表来自「设置 › GeoData」。":   "寫 GeoIP 清單的名稱，清單裡的位址都走這個路由組。清單來自「設定 › GeoData」。",
}

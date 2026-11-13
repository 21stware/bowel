package i18n

// The strings of handing the core over to the next version, kept apart
// from the main tables so that work on other parts merges cleanly.

func init() {
	for k, v := range jaHandover {
		jaTable[k] = v
	}
	for k, v := range hantHandover {
		hantTable[k] = v
	}
}

var jaHandover = map[string]string{
	"bowel 更新后没有重新打开，代理还在按更新前的配置运行。\n\n重新打开 bowel 就能接着用。要停止代理并还原系统的网络设置吗？停止后流量不再经过代理。": "bowel はアップデート後に再び開かれず、プロキシはアップデート前の設定のまま動作しています。\n\nbowel をもう一度開けばそのまま使えます。プロキシを停止して、システムのネットワーク設定を元に戻しますか？停止すると通信はプロキシを経由しなくなります。",
	"选「是」停止代理，选「否」让它继续运行。": "「はい」でプロキシを停止、「いいえ」でそのまま動作を続けます。",
	"停止代理": "プロキシを停止",
	"继续代理": "このまま続ける",
}

var hantHandover = map[string]string{
	"bowel 更新后没有重新打开，代理还在按更新前的配置运行。\n\n重新打开 bowel 就能接着用。要停止代理并还原系统的网络设置吗？停止后流量不再经过代理。": "bowel 更新後沒有重新打開，代理仍在按更新前的設定執行。\n\n重新打開 bowel 就能接著用。要停止代理並還原系統的網路設定嗎？停止後流量不再經過代理。",
	"选「是」停止代理，选「否」让它继续运行。": "選「是」停止代理，選「否」讓它繼續執行。",
	"停止代理": "停止代理",
	"继续代理": "繼續代理",
}

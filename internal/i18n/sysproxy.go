package i18n

// The strings of what Windows refuses of the system proxy, kept apart from
// the main tables so that work on other parts merges cleanly.

func init() {
	for k, v := range jaSysproxy {
		jaTable[k] = v
	}
	for k, v := range hantSysproxy {
		hantTable[k] = v
	}
}

var jaSysproxy = map[string]string{
	"被拒绝的内容在日志和 bowel status --json 的 takeover.written 里。":        "拒否された内容は、ログと bowel status --json の takeover.written にあります。",
	"Windows 不接受代理地址 ProxyServer=%q（flags=%#x，不写「不走代理」的列表也一样）：%w": "Windows がプロキシのアドレス ProxyServer=%q を受け付けません（flags=%#x、「プロキシ除外」のリストを書かなくても同じ）：%w",
	"Windows 不接受这次写入的代理设置（%s）：%w":                                 "Windows が今回書き込んだプロキシ設定（%s）を受け付けません：%w",
	"Windows 没有接受「不走代理」的列表（ProxyOverride），已改用只含本机和局域网的列表":         "Windows が「プロキシ除外」のリスト（ProxyOverride）を受け付けなかったため、このコンピューターとローカルネットワークだけのリストに切り替えました",
	"Windows 没有接受「不走代理」的列表（ProxyOverride），系统里原有的列表没有改动":           "Windows が「プロキシ除外」のリスト（ProxyOverride）を受け付けなかったため、システムに元からあるリストはそのままです",
}

var hantSysproxy = map[string]string{
	"被拒绝的内容在日志和 bowel status --json 的 takeover.written 里。":        "被拒絕的內容在日誌和 bowel status --json 的 takeover.written 裡。",
	"Windows 不接受代理地址 ProxyServer=%q（flags=%#x，不写「不走代理」的列表也一样）：%w": "Windows 不接受代理位址 ProxyServer=%q（flags=%#x，不寫「不走代理」的清單也一樣）：%w",
	"Windows 不接受这次写入的代理设置（%s）：%w":                                 "Windows 不接受這次寫入的代理設定（%s）：%w",
	"Windows 没有接受「不走代理」的列表（ProxyOverride），已改用只含本机和局域网的列表":         "Windows 沒有接受「不走代理」的清單（ProxyOverride），已改用只含本機和區域網路的清單",
	"Windows 没有接受「不走代理」的列表（ProxyOverride），系统里原有的列表没有改动":           "Windows 沒有接受「不走代理」的清單（ProxyOverride），系統裡原有的清單沒有改動",
}

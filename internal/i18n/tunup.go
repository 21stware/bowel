package i18n

// The strings of a TUN that is wanted and that the core did not bring
// up, kept apart so that work on other parts merges cleanly.

func init() {
	for k, v := range jaTunUp {
		jaTable[k] = v
	}
	for k, v := range hantTunUp {
		hantTable[k] = v
	}
}

var jaTunUp = map[string]string{
	"TUN 没有生效": "TUN が機能していません",
	"内核没有把 TUN 设备建起来，流量没有被 TUN 接管；原因在 bowel logs 里找「Start TUN listening error」。关掉再打开 TUN 会重试": "コアが TUN デバイスを作成できなかったため、通信は TUN を通っていません。理由は bowel logs で「Start TUN listening error」を探してください。TUN をオフにしてからオンにすると再試行します",
	"内核没有把 TUN 设备建起来，流量没有被 TUN 接管：%s。关掉再打开 TUN 会重试":                                           "コアが TUN デバイスを作成できなかったため、通信は TUN を通っていません：%s。TUN をオフにしてからオンにすると再試行します",
}

var hantTunUp = map[string]string{
	"TUN 没有生效": "TUN 沒有生效",
	"内核没有把 TUN 设备建起来，流量没有被 TUN 接管；原因在 bowel logs 里找「Start TUN listening error」。关掉再打开 TUN 会重试": "核心沒有把 TUN 裝置建起來，流量沒有被 TUN 接管；原因在 bowel logs 裡找「Start TUN listening error」。關掉再開啟 TUN 會重試",
	"内核没有把 TUN 设备建起来，流量没有被 TUN 接管：%s。关掉再打开 TUN 会重试":                                           "核心沒有把 TUN 裝置建起來，流量沒有被 TUN 接管：%s。關掉再開啟 TUN 會重試",
}

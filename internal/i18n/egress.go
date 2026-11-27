package i18n

// The strings of egress for other containers (app_container_egress.go),
// kept apart so that work on other parts merges cleanly.

func init() {
	for k, v := range jaEgress {
		jaTable[k] = v
	}
	for k, v := range hantEgress {
		hantTable[k] = v
	}
}

var jaEgress = map[string]string{
	"TUN 没有在转发流量：与 bowel 共用网络的容器会直接连出去，从端口进来的流量仍按出口锁定走。": "TUN がトラフィックを転送していないため、bowel とネットワークを共有するコンテナは直接接続します。bowel のポートから入るトラフィックは引き続き出口ロックに従います。",
}

var hantEgress = map[string]string{
	"TUN 没有在转发流量：与 bowel 共用网络的容器会直接连出去，从端口进来的流量仍按出口锁定走。": "TUN 沒有在轉送流量：與 bowel 共用網路的容器會直接連出去，從連接埠進來的流量仍按出口鎖定走。",
}

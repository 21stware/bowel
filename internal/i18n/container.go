package i18n

// The strings of bowel in a container (app_container.go) and of TUN and
// the gateway run as root there, kept apart so that work on other parts
// merges cleanly.

func init() {
	for k, v := range jaContainer {
		jaTable[k] = v
	}
	for k, v := range hantContainer {
		hantTable[k] = v
	}
}

var jaContainer = map[string]string{
	"没有 /dev/net/tun，TUN 需要这个设备。在容器里运行时，请加上 --device /dev/net/tun 和 --cap-add NET_ADMIN":                             "/dev/net/tun がありません。TUN にはこのデバイスが必要です。コンテナで動かすときは --device /dev/net/tun と --cap-add NET_ADMIN を付けてください",
	"打不开 /dev/net/tun，TUN 需要这个设备。在容器里运行时，请加上 --device /dev/net/tun 和 --cap-add NET_ADMIN：":                           "/dev/net/tun を開けません。TUN にはこのデバイスが必要です。コンテナで動かすときは --device /dev/net/tun と --cap-add NET_ADMIN を付けてください：",
	"bowel 以 root 运行，但没有 NET_ADMIN 权限，TUN 建不起来。在容器里运行时，请加上 --cap-add NET_ADMIN 和 --device /dev/net/tun":              "bowel は root で動いていますが NET_ADMIN 権限がないため、TUN を作成できません。コンテナで動かすときは --cap-add NET_ADMIN と --device /dev/net/tun を付けてください",
	"/proc/sys 是只读的，bowel 没法打开 IP 转发（容器里常见）。请用 --sysctl net.ipv4.ip_forward=1 启动容器；用 --network host 时，在宿主机上打开 IP 转发": "/proc/sys が読み取り専用のため、bowel は IP 転送をオンにできません（コンテナではよくあります）。--sysctl net.ipv4.ip_forward=1 を付けてコンテナを起動してください。--network host のときはホストで IP 転送をオンにしてください",
	"容器里没有系统代理可设：请让程序直接使用代理端口，或用 TUN":                                                                                "コンテナには設定できるシステムプロキシがありません。プログラムにプロキシのポートを直接使わせるか、TUN を使ってください",
	"内核没有运行（%s）":         "コアが動いていません（%s）",
	"内核在运行，但端口 %d 没有应答":  "コアは動いていますが、ポート %d が応答しません",
	"正常：内核 %s 在运行，端口 %d": "正常：コア %s が動作中、ポート %d",
	"健康检查：bowel 在应答、内核在运行并且代理端口有应答时退出码为 0，否则为 1": "ヘルスチェック：bowel が応答し、コアが動いてプロキシのポートが応答すれば終了コード 0、そうでなければ 1",
}

var hantContainer = map[string]string{
	"没有 /dev/net/tun，TUN 需要这个设备。在容器里运行时，请加上 --device /dev/net/tun 和 --cap-add NET_ADMIN":                             "沒有 /dev/net/tun，TUN 需要這個裝置。在容器裡執行時，請加上 --device /dev/net/tun 和 --cap-add NET_ADMIN",
	"打不开 /dev/net/tun，TUN 需要这个设备。在容器里运行时，请加上 --device /dev/net/tun 和 --cap-add NET_ADMIN：":                           "打不開 /dev/net/tun，TUN 需要這個裝置。在容器裡執行時，請加上 --device /dev/net/tun 和 --cap-add NET_ADMIN：",
	"bowel 以 root 运行，但没有 NET_ADMIN 权限，TUN 建不起来。在容器里运行时，请加上 --cap-add NET_ADMIN 和 --device /dev/net/tun":              "bowel 以 root 執行，但沒有 NET_ADMIN 權限，TUN 建不起來。在容器裡執行時，請加上 --cap-add NET_ADMIN 和 --device /dev/net/tun",
	"/proc/sys 是只读的，bowel 没法打开 IP 转发（容器里常见）。请用 --sysctl net.ipv4.ip_forward=1 启动容器；用 --network host 时，在宿主机上打开 IP 转发": "/proc/sys 是唯讀的，bowel 沒辦法開啟 IP 轉送（容器裡常見）。請用 --sysctl net.ipv4.ip_forward=1 啟動容器；用 --network host 時，在主機上開啟 IP 轉送",
	"容器里没有系统代理可设：请让程序直接使用代理端口，或用 TUN":                                                                                "容器裡沒有系統代理可設定：請讓程式直接使用代理連接埠，或用 TUN",
	"内核没有运行（%s）":         "核心沒有執行（%s）",
	"内核在运行，但端口 %d 没有应答":  "核心在執行，但連接埠 %d 沒有回應",
	"正常：内核 %s 在运行，端口 %d": "正常：核心 %s 在執行，連接埠 %d",
	"健康检查：bowel 在应答、内核在运行并且代理端口有应答时退出码为 0，否则为 1": "健康檢查：bowel 有回應、核心在執行且代理連接埠有回應時結束代碼為 0，否則為 1",
}

package i18n

// The strings of letting Windows' packaged (UWP) apps reach the system
// proxy, kept apart so that work on other parts merges cleanly.

func init() {
	for k, v := range jaUWP {
		jaTable[k] = v
	}
	for k, v := range hantUWP {
		hantTable[k] = v
	}
}

var jaUWP = map[string]string{
	"允许 UWP 应用使用代理": "UWP アプリにプロキシの使用を許可",
	"微软商店、邮件、Xbox 等 UWP 应用默认连不上本机的代理；允许时 Windows 可能要管理员授权": "Microsoft Store、メール、Xbox などの UWP アプリは、既定ではこのコンピューター上のプロキシに接続できません。許可するとき Windows が管理者の承認を求めることがあります",
	"这台电脑上的 %d 个 UWP 应用都可以使用代理":                            "このコンピューターの UWP アプリ %d 個すべてがプロキシを使用できます",
	"（已允许 %d / %d 个）":      "（%d / %d 個を許可済み）",
	"全部允许":                 "すべて許可",
	"UWP 应用现在可以使用代理了":      "UWP アプリがプロキシを使用できるようになりました",
	"已撤销 bowel 允许的 UWP 应用": "bowel が許可した UWP アプリを元に戻しました",
	"有 %d 个应用没能加入例外":       "%d 個のアプリを例外に追加できませんでした",
	"有 %d 个应用没能移出例外":       "%d 個のアプリを例外から外せませんでした",
	"读不到应用容器的列表：%s":        "アプリコンテナーの一覧を読み取れません: %s",
	"读不到本机回环的例外列表：%s":      "ループバックの例外一覧を読み取れません: %s",
}

var hantUWP = map[string]string{
	"允许 UWP 应用使用代理": "允許 UWP 應用程式使用代理",
	"微软商店、邮件、Xbox 等 UWP 应用默认连不上本机的代理；允许时 Windows 可能要管理员授权": "Microsoft Store、郵件、Xbox 等 UWP 應用程式預設連不上本機的代理；允許時 Windows 可能需要管理員授權",
	"这台电脑上的 %d 个 UWP 应用都可以使用代理":                            "這台電腦上的 %d 個 UWP 應用程式都可以使用代理",
	"（已允许 %d / %d 个）":      "（已允許 %d / %d 個）",
	"全部允许":                 "全部允許",
	"UWP 应用现在可以使用代理了":      "UWP 應用程式現在可以使用代理了",
	"已撤销 bowel 允许的 UWP 应用": "已撤銷 bowel 允許的 UWP 應用程式",
	"有 %d 个应用没能加入例外":       "有 %d 個應用程式沒能加入例外",
	"有 %d 个应用没能移出例外":       "有 %d 個應用程式沒能移出例外",
	"读不到应用容器的列表：%s":        "讀不到應用程式容器的清單：%s",
	"读不到本机回环的例外列表：%s":      "讀不到本機回環的例外清單：%s",
}

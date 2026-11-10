package i18n

// The strings of what Linux and Windows do their own way (TUN's
// installation, the gateway, the system proxy), kept apart from the main
// tables so that work on other parts merges cleanly.

func init() {
	for k, v := range jaPlatforms {
		jaTable[k] = v
	}
	for k, v := range hantPlatforms {
		hantTable[k] = v
	}
}

var jaPlatforms = map[string]string{
	"Linux 的网卡名最长 15 个字符，不能有空格和斜杠":             "Linux のデバイス名は 15 文字までで、空白とスラッシュは使えません",
	"无法通过计划任务启动内核（重新开启 TUN 可以再授权一次）：%s":        "タスクスケジューラからコアを起動できませんでした（TUN をもう一度オンにすると再度許可を求めます）：%s",
	"计划任务没有把内核启动起来":                            "スケジュールされたタスクがコアを起動しませんでした",
	"%s 所在的文件系统以 nosuid 挂载，内核在这里拿不到 TUN 需要的权限": "%s のファイルシステムは nosuid でマウントされているため、コアは TUN に必要な権限を得られません",
	"找不到 pkexec：请先安装 polkit，授权要通过它来完成":         "pkexec が見つかりません。許可に必要な polkit をインストールしてください",
	"无法取得授权（桌面可能没有运行 polkit 认证代理）：%s":          "許可を得られませんでした（デスクトップで polkit 認証エージェントが動いていない可能性があります）：%s",
	"授权失败：没有启动提权的进程":                           "許可に失敗しました：昇格したプロセスが起動しませんでした",
	"授权后的安装没有完成（退出码 %d）":                       "許可後のインストールが完了しませんでした（終了コード %d）",
	"路径里有不能使用的字符：%s":                           "パスに使用できない文字があります：%s",
}

var hantPlatforms = map[string]string{
	"Linux 的网卡名最长 15 个字符，不能有空格和斜杠":             "Linux 的網路卡名稱最長 15 個字元，不能有空格和斜線",
	"无法通过计划任务启动内核（重新开启 TUN 可以再授权一次）：%s":        "無法透過排程工作啟動核心（重新開啟 TUN 可以再授權一次）：%s",
	"计划任务没有把内核启动起来":                            "排程工作沒有把核心啟動起來",
	"%s 所在的文件系统以 nosuid 挂载，内核在这里拿不到 TUN 需要的权限": "%s 所在的檔案系統以 nosuid 掛載，核心在這裡拿不到 TUN 需要的權限",
	"找不到 pkexec：请先安装 polkit，授权要通过它来完成":         "找不到 pkexec：請先安裝 polkit，授權要透過它來完成",
	"无法取得授权（桌面可能没有运行 polkit 认证代理）：%s":          "無法取得授權（桌面可能沒有執行 polkit 認證代理）：%s",
	"授权失败：没有启动提权的进程":                           "授權失敗：沒有啟動提權的行程",
	"授权后的安装没有完成（退出码 %d）":                       "授權後的安裝沒有完成（結束代碼 %d）",
	"路径里有不能使用的字符：%s":                           "路徑裡有不能使用的字元：%s",
}

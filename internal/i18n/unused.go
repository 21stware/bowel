package i18n

// The strings about what of an imported config is not used
// (core.Imported.Unused), kept apart from the main tables so that work on
// other parts merges cleanly.

func init() {
	for k, v := range jaUnused {
		jaTable[k] = v
	}
	for k, v := range hantUnused {
		hantTable[k] = v
	}
}

var jaUnused = map[string]string{
	"这份配置里的 %s 不会生效：DNS、TUN 和本机端口用的是 bowel 自己的设置。": "この設定の %s は使われません。DNS、TUN、ローカルのポートは bowel 自身の設定に従います。",
}

var hantUnused = map[string]string{
	"这份配置里的 %s 不会生效：DNS、TUN 和本机端口用的是 bowel 自己的设置。": "這份設定裡的 %s 不會生效：DNS、TUN 和本機連接埠用的是 bowel 自己的設定。",
}

package i18n

// The strings of removing single nodes from a Provider (#160), kept apart
// from the main tables so that work on other parts merges cleanly.

func init() {
	for k, v := range jaHideNodes {
		jaTable[k] = v
	}
	for k, v := range hantHideNodes {
		hantTable[k] = v
	}
}

var jaHideNodes = map[string]string{
	"节点名为空":                 "ノード名が空です",
	"「%s」不是来自 Provider 的节点": "「%s」は Provider のノードではありません",
	"移除这个节点":                "このノードを削除",
	"已移除 %s，订阅更新后也不会回来":     "%s を削除しました。サブスクリプションを更新しても戻りません",
	"。手动选了它的路由组：%s":         "。手動で選んでいたルートグループ：%s",
	"已移除的节点 · %d":           "削除したノード · %d",
	"恢复":                    "元に戻す",
}

var hantHideNodes = map[string]string{
	"节点名为空":                 "節點名稱為空",
	"「%s」不是来自 Provider 的节点": "「%s」不是來自 Provider 的節點",
	"移除这个节点":                "移除這個節點",
	"已移除 %s，订阅更新后也不会回来":     "已移除 %s，訂閱更新後也不會回來",
	"。手动选了它的路由组：%s":         "。手動選了它的路由組：%s",
	"已移除的节点 · %d":           "已移除的節點 · %d",
	"恢复":                    "恢復",
}

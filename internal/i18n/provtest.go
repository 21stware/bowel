package i18n

// The strings of testing a Provider's nodes from its row, kept apart from
// the main tables so that work on other parts merges cleanly.

func init() {
	for k, v := range jaProvTest {
		jaTable[k] = v
	}
	for k, v := range hantProvTest {
		hantTable[k] = v
	}
}

var jaProvTest = map[string]string{
	"这个来源没有可测的节点":         "このソースにはテストできるノードがありません",
	"测这个来源每个节点的 Ping 和延迟": "このソースの全ノードの Ping と遅延をテスト",
}

var hantProvTest = map[string]string{
	"这个来源没有可测的节点":         "這個來源沒有可測的節點",
	"测这个来源每个节点的 Ping 和延迟": "測這個來源每個節點的 Ping 和延遲",
}

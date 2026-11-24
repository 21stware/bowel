package i18n

// The strings of a test site turning a node away (routes.Expected), kept
// apart from the main tables so that work on other parts merges cleanly.
// The word for it in a node's row, 被拒绝, is the one mcp.go already has.

func init() {
	for k, v := range jaRefused {
		jaTable[k] = v
	}
	for k, v := range hantRefused {
		hantTable[k] = v
	}
}

var jaRefused = map[string]string{
	"%s：%s 被 %s 拒绝": "%s：%s は %s に拒否されました",
}

var hantRefused = map[string]string{
	"%s：%s 被 %s 拒绝": "%s：%s 被 %s 拒絕",
}

package web

// Simplified Chinese text of the Logs tab of a website.
func init() {
	for k, v := range map[string]string{
		"tab.logs":        "日志",
		"log.title":       "网站日志",
		"log.sub":         "最新的日志行，最新的在底部。仅供查看。",
		"log.kinds":       "日志类型",
		"log.kind.access": "访问",
		"log.kind.error":  "错误",
		"log.kind.app":    "应用",
		"log.search":      "仅显示包含以下内容的行",
		"log.search.ph":   "例如 404、IP 地址或路径",
		"log.lines":       "行数",
		"log.show":        "查看",
		"log.size":        "整个文件大小",
		"log.matched":     "匹配的行数",
		"log.truncated":   "文件较大，仅搜索末尾部分",
		"log.view":        "日志内容",
		"log.missing":     "尚无 %s 的日志。网站收到第一个请求时会自动创建。",
		"log.empty":       "日志为空。",
		"log.nomatch":     "此日志末尾没有包含该文本的行。",
		"log.hint":        "访问日志记录每个发往网站的请求，错误日志记录 nginx 的错误。Node 或 Python 网站的应用日志来自网站服务。日志每天轮转，保留 14 份。",
		"log.err.query":   "搜索文本无效，未应用筛选。",
	} {
		zh[k] = v
	}
}

package web

// Simplified Chinese text of the site list button and the cards at the end of the Overview tab.
func init() {
	for k, v := range map[string]string{
		"sites.manage": "管理",
		"ov.del.title": "删除网站",
		"ov.del.d":     "永久删除该网站的文件、数据库、系统账户和配置。此操作无法撤销。",
		"ov.del.btn":   "删除网站",
		"ps.title":     "网站加速",
		"ps.sub":       "在服务器上自动压缩 HTML、CSS 和 JS，并优化图片，无需修改代码即可加快页面加载。",
		"ps.on":        "已启用",
		"ps.off":       "已禁用",
		"ps.gzip":      "发送给浏览器前压缩 HTML、CSS 和 JS",
		"ps.cache":     "优化并转换图片（PNG 转 JPEG/WebP）以减小体积",
		"ps.btn.on":    "启用加速",
		"ps.btn.off":   "禁用加速",
		"ps.note":      "优化后的文件会被缓存。首次请求可能较慢，之后的请求会更快。",
		"ps.custom":    "该网站在 vHost 标签页中使用了自定义 nginx 配置。请在那里添加 pagespeed 指令，或恢复默认配置。",
		"act.speed":    "修改网站 %s 的加速设置",
	} {
		zh[k] = v
	}
}

package web

// Simplified Chinese text of the web mail card of a website.
func init() {
	for k, v := range map[string]string{
		"act.webmail":  "修改网站 %s 的 webmail",
		"wm.title":     "本域名的 Webmail",
		"wm.sub":       "在本网站的 /webmail/ 路径下收发邮件，无需额外域名或证书，子域名同样可用。",
		"wm.on":        "已启用",
		"wm.off":       "已禁用",
		"wm.btn.on":    "启用 webmail",
		"wm.btn.off":   "禁用 webmail",
		"wm.note":      "使用完整邮箱地址和邮箱密码，在任意域名下登录。Webmail 与网站共用同一来源：网站若存在漏洞，攻击者可能借此读取邮件。请仅在可信任的网站上启用。",
		"wm.needs.ssl": "请先为本网站启用 SSL，密码不能通过不安全的连接输入。",
		"wm.custom":    "本网站使用自定义 vHost，面板无法自动添加。请手动将 /webmail/ 配置添加到 vHost 中。",
		"wm.err.ssl":   "请先启用 SSL：webmail 只能通过 HTTPS 运行。",
		"wm.err.off":   "请先在邮件页面启用邮件服务器和 webmail。",
		"wm.err.old":   "已安装的 webmail 版本不支持 /webmail/。请在邮件页面禁用后重新启用 webmail 以更新。",
	} {
		zh[k] = v
	}
}

package web

// Simplified Chinese text of the page that adds the DNS records of a mail domain to Cloudflare.
func init() {
	for k, v := range map[string]string{
		"page.emailcf":          "通过 Cloudflare 添加 DNS",
		"act.mail.cf":           "将邮件域名 %s 的 DNS 记录添加到 Cloudflare",
		"mail.cf.btn":           "通过 Cloudflare 添加 DNS",
		"mail.cf.back":          "返回邮箱",
		"mail.cf.title":         "将 DNS 记录添加到 Cloudflare",
		"mail.cf.sub":           "域名 %s 使用 Cloudflare 的域名服务器。面板可以替你添加下面的记录，你无需手动复制。",
		"mail.cf.recs":          "将要添加的记录",
		"mail.cf.hostnote":      "若邮件服务器主机名 %s 尚无 A 记录，且本机有公网 IP，面板还会添加一条 A 记录（不经过 Cloudflare 代理，因为邮件不走代理）。已有的记录保持不变。",
		"mail.cf.token.title":   "允许面板编辑此域名的 DNS",
		"mail.cf.step1.a":       "打开 Cloudflare 的 API 令牌创建页面：",
		"mail.cf.step2":         "选择 Create Token，然后使用“Edit zone DNS”模板。",
		"mail.cf.step3":         "在 Zone Resources 下选择 zone %s。所需权限：Zone · DNS · Edit 和 Zone · Zone · Read。",
		"mail.cf.step4":         "创建令牌后粘贴到下方输入框。使用完毕后，请在 Cloudflare 中删除该令牌。",
		"mail.cf.token":         "API 令牌",
		"mail.cf.token.hint":    "仅用于本次操作，不会保存在服务器上。",
		"mail.cf.add":           "添加记录",
		"mail.cf.zone":          "Zone：%s",
		"mail.cf.st.added":      "已添加",
		"mail.cf.st.exists":     "已存在",
		"mail.cf.st.conflict":   "与现有记录冲突",
		"mail.cf.st.failed":     "失败",
		"mail.cf.nohostip":      "未检测到公网 IP，未添加 %s 的 A 记录。请手动添加一条指向本机地址的 A 记录。",
		"mail.cf.conflict":      "已有记录与之冲突。面板不会覆盖或删除它。如需修改，请在 Cloudflare 中手动检查并修改。",
		"mail.cf.err.token":     "API 令牌格式不正确。令牌由 20 到 128 个字母、数字、下划线或连字符组成。",
		"mail.cf.err.nodomain":  "该域名尚未添加到邮件服务器。",
		"mail.cf.err.badtoken":  "Cloudflare 不接受此 API 令牌，请检查令牌。",
		"mail.cf.err.noperm":    "令牌权限不足。请为正确的 zone 授予 Zone · DNS · Edit 和 Zone · Zone · Read。",
		"mail.cf.err.nozone":    "该令牌无法访问此域名的任何 zone。创建令牌时请选择正确的 zone。",
		"mail.cf.err.ratelimit": "已达到 Cloudflare 的请求频率限制，请一分钟后重试。",
		"mail.cf.err.network":   "本服务器无法连接到 Cloudflare。",
		"mail.cf.err.off":       "请先启用邮件服务器。",
	} {
		zh[k] = v
	}
}

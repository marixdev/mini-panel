package web

// Simplified Chinese text of the shell of every page: the name, the navigation, the sign-in form and the errors of the first-run page.
func init() {
	for k, v := range map[string]string{
		"app.name":         "Mini Panel",
		"theme.toggle":     "切换浅色/深色主题",
		"nav.main":         "主导航",
		"nav.skip":         "跳到内容",
		"login.title":      "登录",
		"login.wrong":      "用户名或密码错误。",
		"login.locked":     "登录失败次数过多，请 %d 秒后再试。",
		"page.login":       "登录",
		"page.setup":       "设置 Mini Panel",
		"page.notfound":    "页面不存在",
		"setup.err.user":   "用户名无效。只能使用小写字母、数字、连字符或下划线，且必须以字母开头。",
		"setup.err.short":  "密码至少需要 %d 个字符。",
		"setup.err.long":   "密码太长（最多 256 个字符）。",
		"setup.err.match":  "两次输入的密码不一致。",
		"setup.err.hash":   "无法加密密码，请重试。",
		"setup.err.nosave": "无法保存账户：尚未配置账户存储。",
		"setup.err.write":  "无法保存账户：%s",
	} {
		zh[k] = v
	}
}

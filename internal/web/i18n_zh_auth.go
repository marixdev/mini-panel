package web

// Simplified Chinese text of the sign-in, first-run and not-found pages.
func init() {
	for k, v := range map[string]string{
		"notfound.description":     "您打开的地址不存在或已变更。请检查网址，或返回首页继续操作。",
		"setup.description":        "这是登录 Mini Panel 的唯一账户。请选择您要使用的用户名和密码。",
		"setup.password.hint":      "至少 12 个字符。可以使用一句话作为密码。",
		"login.code.hint":          "请输入验证器应用中显示的 6 位数字。",
		"login.description":        "请使用管理员账户继续。",
		"notfound.document.title":  "页面不存在 - Mini Panel",
		"setup.username.hint":      "仅限小写字母、数字和连字符",
		"setup.submit":             "创建账户并登录",
		"notfound.heading":         "找不到此页面",
		"login.document.title":     "登录 - Mini Panel",
		"setup.heading":            "创建您的账户。",
		"foot.cpu":                 "%s CPU",
		"foot.ram":                 "%s RAM",
		"login.heading":            "登录您的面板。",
		"setup.document.title":     "设置 Mini Panel",
		"notfound.back":            "返回上一页",
		"setup.badge":              "初始设置",
		"login.code.heading":       "输入验证码。",
		"setup.password.confirm":   "确认密码",
		"notfound.home":            "返回首页",
		"auth.username":            "用户名",
		"auth.password.show.label": "显示密码",
		"auth.skip":                "跳到表单",
		"auth.code":                "验证码",
		"auth.signin":              "登录",
		"auth.confirm":             "确认",
		"auth.password":            "密码",
		"auth.password.show":       "显示",
		"auth.password.hide":       "隐藏",
		"language.label":           "语言",
	} {
		zh[k] = v
	}
}

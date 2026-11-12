package web

// Simplified Chinese text of the terminal.
func init() {
	for k, v := range map[string]string{
		"tab.terminal":    "终端",
		"term.hint":       "以网站账户身份登录。面板不会保存你在此处输入或看到的任何内容。",
		"term.connecting": "正在连接…",
		"term.connected":  "已连接",
		"term.closed":     "会话已结束",
		"term.failed":     "连接失败",
		"term.off":        "请先在该标签页中为此网站启用 SSH/SFTP 访问",
		"term.loginfail":  "无法以网站账户登录。请到 SSH/SFTP 标签页检查面板密钥。",
		"term.toomany":    "打开的终端过多，请关闭一些后重试。",
	} {
		zh[k] = v
	}
}

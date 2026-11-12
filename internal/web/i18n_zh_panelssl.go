package web

// Simplified Chinese text of the certificate card of the security page: the certificate of the panel and of the database browser, which share one.
func init() {
	for k, v := range map[string]string{
		"psl.title":            "面板 SSL 证书",
		"psl.sub":              "面板和 phpMyAdmin 共用一个证书。通过域名访问时，两个端口都不会再出现浏览器警告。",
		"psl.mode.selfsigned":  "自签名",
		"psl.mode.letsencrypt": "Let's Encrypt",
		"psl.mode.custom":      "自定义证书",
		"psl.selfsigned.d":     "面板当前使用自签名证书，浏览器会显示不安全警告。请将一个域名指向本服务器，然后在下方签发证书。",
		"psl.domain":           "域名",
		"psl.urls":             "访问地址",
		"psl.panel":            "面板",
		"psl.pma":              "phpMyAdmin",
		"psl.notcover":         "当前证书不包含此域名。",
		"psl.auto":             "Let's Encrypt 证书会在到期前 30 天自动续期，使用网站的续期定时任务。",
		"psl.manual":           "自定义证书不会自动续期，请在到期前更换。",
		"psl.le.title":         "使用 Let's Encrypt 签发",
		"psl.le.desc":          "免费。面板会检查 DNS 解析，在 80 端口开启一个 nginx server 块供 Let's Encrypt 验证，然后为两个端口安装证书。",
		"psl.le.btn":           "签发证书",
		"psl.le.replace":       "重新签发",
		"psl.le.hint":          "请将域名的 A 记录指向 %s，并在服务商防火墙中开放 80 端口。面板请使用单独的域名，不要使用网站的域名。",
		"psl.own.title":        "使用自定义证书",
		"psl.own.desc":         "粘贴已购买或从其他途径获得的证书，证书必须覆盖上方的域名。",
		"psl.own.btn":          "安装证书",
		"psl.relogin":          "修改后，请使用新域名打开面板并重新登录。登录会话与创建时的地址绑定。",
		"psl.off.title":        "恢复为自签名证书",
		"psl.off.warn":         "面板将不再响应此域名，仅可通过 IP 地址访问，并使用自签名证书。",
		"psl.off.btn":          "使用自签名证书",
		"psl.fail":             "操作未完成",
		"psl.err.domain":       "请输入包含点号的小写域名，例如 panel.example.com。不支持 IP 地址，无法为 IP 签发证书。",
		"psl.err.nokey":        "请粘贴证书的私钥。",
		"psl.err.site":         "该域名已用于某个网站。面板需要一个单独的域名，例如 panel.example.com。",
		"psl.err.dns":          "域名尚未指向本服务器，Let's Encrypt 无法验证。请检查 A 记录，几分钟后重试。",
		"act.psl.le":           "为面板签发 Let's Encrypt 证书",
		"act.psl.custom":       "为面板安装自定义证书",
		"act.psl.off":          "恢复面板的自签名证书",
	} {
		zh[k] = v
	}
}

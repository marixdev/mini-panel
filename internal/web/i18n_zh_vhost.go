package web

// Simplified Chinese text of the vHost tab.
func init() {
	for k, v := range map[string]string{
		"tab.vhost":        "vHost",
		"vh.title":         "网站 nginx 配置",
		"vh.sub":           "直接编辑此网站的 nginx server 块。面板会先运行 nginx -t 检查，再加载配置。",
		"vh.custom.badge":  "自定义配置",
		"vh.default.badge": "面板生成",
		"vh.custom.warn":   "此网站正在使用您自行编写的配置。在恢复为默认配置之前，域名、SSL 启用或取消以及跳转的修改都不会自动同步到这里，面板也会拒绝这些操作。Let's Encrypt 续期仍然正常工作。",
		"vh.default.hint":  "这是面板根据网站的域名、SSL 和设置生成的配置。保存修改后，网站将改用您的自定义版本。",
		"vh.text":          "配置",
		"vh.hint":          "最大 %d KB。如果 nginx -t 检查失败，将保留正在运行的配置。",
		"vh.hint.ssl":      "网站已启用 SSL：请保留 location /.well-known/acme-challenge/ 段，以便 Let's Encrypt 证书能够续期。",
		"vh.save":          "保存并应用",
		"vh.reset.title":   "恢复默认配置",
		"vh.reset.warn":    "您自定义的配置将被丢弃，并根据当前设置重新生成。请先将正在编辑的内容复制到别处。",
		"vh.reset.btn":     "恢复默认",
		"act.vhost.save":   "保存网站 %s 的 nginx 配置",
		"act.vhost.reset":  "重置网站 %s 的 nginx 配置",
		"vh.fail":          "配置未保存",
		"vh.err.check":     "配置无效：%s。",
		"vh.err.nginx":     "无法应用。已保留正在运行的配置，nginx 的错误信息见下方结果。",
		"vh.err.custom":    "网站正在使用自定义 nginx 配置，此操作不可用。请先在 vHost 选项卡中恢复默认配置。",
	} {
		zh[k] = v
	}
}

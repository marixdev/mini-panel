package web

// English text of the vHost tab.
func init() {
	for k, v := range map[string]string{
		"tab.vhost":        "vHost",
		"vh.title":         "Website nginx configuration",
		"vh.sub":           "Edit this website's nginx server block directly. The panel runs nginx -t before loading it.",
		"vh.custom.badge":  "Custom configuration",
		"vh.default.badge": "Panel generated",
		"vh.custom.warn":   "This website uses your custom configuration. Domain, SSL and redirect changes are refused until you restore the default. Let's Encrypt renewal still works.",
		"vh.default.hint":  "The panel generates this configuration from the website's domain, SSL and settings. Saving edits switches the website to your custom version.",
		"vh.text":          "Configuration",
		"vh.hint":          "Maximum %d KB. If nginx -t fails, the running configuration is kept.",
		"vh.hint.ssl":      "SSL is enabled: keep location /.well-known/acme-challenge/ so Let's Encrypt can renew the certificate.",
		"vh.save":          "Save and apply",
		"vh.reset.title":   "Restore default configuration",
		"vh.reset.warn":    "Your custom configuration will be discarded and regenerated from the current settings. Copy your edits elsewhere first.",
		"vh.reset.btn":     "Restore default",
		"act.vhost.save":   "Save nginx configuration for website %s",
		"act.vhost.reset":  "Reset nginx configuration for website %s",
		"vh.fail":          "Configuration not saved",
		"vh.err.check":     "Invalid configuration: %s.",
		"vh.err.nginx":     "Could not apply. The running configuration was kept; see nginx errors below.",
		"vh.err.custom":    "This operation is unavailable while the website uses a custom nginx configuration. Restore the default in the vHost tab first.",
	} {
		en[k] = v
	}
}

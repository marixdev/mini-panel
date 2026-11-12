package web

// English text of the site list button and the cards at the end of the Overview tab.
func init() {
	for k, v := range map[string]string{
		"sites.manage": "Manage",
		"ov.del.title": "Delete website",
		"ov.del.d":     "Permanently delete this website's files, databases, system account and configuration. This cannot be undone.",
		"ov.del.btn":   "Delete website",
		"ps.title":     "Website acceleration",
		"ps.sub":       "Automatically minify HTML, CSS and JS and optimize images on the server for faster pages without changing your code.",
		"ps.on":        "Enabled",
		"ps.off":       "Disabled",
		"ps.gzip":      "Minify and compress HTML, CSS and JS before sending to the browser",
		"ps.cache":     "Optimize and convert images (PNG→JPEG/WebP) to reduce size",
		"ps.btn.on":    "Enable acceleration",
		"ps.btn.off":   "Disable acceleration",
		"ps.note":      "Optimized files are cached. The first request may be slower; subsequent requests are faster.",
		"ps.custom":    "This website uses a custom nginx configuration in the vHost tab. Add pagespeed directives there or restore the default configuration.",
		"act.speed":    "Change acceleration settings for website %s",
	} {
		en[k] = v
	}
}

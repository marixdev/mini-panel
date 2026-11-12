package web

// English text of the web mail card of a website.
func init() {
	for k, v := range map[string]string{
		"act.webmail":  "Change webmail for website %s",
		"wm.title":     "Webmail on this domain",
		"wm.sub":       "Read and send mail at this website's /webmail/ path. No extra domain or certificate needed. Works on subdomains too.",
		"wm.on":        "Enabled",
		"wm.off":       "Disabled",
		"wm.btn.on":    "Enable webmail",
		"wm.btn.off":   "Disable webmail",
		"wm.note":      "Sign in with your full email address and mailbox password from any domain. Webmail shares the website's origin: vulnerabilities in the website could expose mail to an attacker. Enable only on trusted websites.",
		"wm.needs.ssl": "Enable SSL for this website first; passwords must not be entered over an insecure connection.",
		"wm.custom":    "This website uses a custom vHost. Add the /webmail/ configuration manually.",
		"wm.err.ssl":   "Enable SSL first: webmail requires HTTPS.",
		"wm.err.off":   "Enable the mail server and webmail on the Email page first.",
		"wm.err.old":   "The installed webmail version does not support /webmail/. Disable and re-enable webmail on the Email page to update it.",
	} {
		en[k] = v
	}
}

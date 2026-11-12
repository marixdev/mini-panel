package web

// English text of the page that adds the DNS records of a mail domain to Cloudflare.
func init() {
	for k, v := range map[string]string{
		"page.emailcf":          "Add DNS through Cloudflare",
		"act.mail.cf":           "Add DNS records for mail domain %s to Cloudflare",
		"mail.cf.btn":           "Add DNS through Cloudflare",
		"mail.cf.back":          "Back to Email",
		"mail.cf.title":         "Add DNS records to Cloudflare",
		"mail.cf.sub":           "Domain %s uses Cloudflare nameservers. The panel can add the records below for you.",
		"mail.cf.recs":          "Records to add",
		"mail.cf.hostnote":      "If mail hostname %s has no A record and this server has a public IP, an unproxied A record is also added. Existing records are kept.",
		"mail.cf.token.title":   "Allow the panel to edit DNS for this domain",
		"mail.cf.step1.a":       "Open Cloudflare's API token creation page:",
		"mail.cf.step2":         "Choose Create Token, then the Edit zone DNS template.",
		"mail.cf.step3":         "Under Zone Resources, select zone %s. Required permissions: Zone · DNS · Edit and Zone · Zone · Read.",
		"mail.cf.step4":         "Create the token and paste it below. Delete it in Cloudflare after use.",
		"mail.cf.token":         "API token",
		"mail.cf.token.hint":    "Used only for this operation and never stored on the server.",
		"mail.cf.add":           "Add records",
		"mail.cf.zone":          "Zone: %s",
		"mail.cf.st.added":      "Added",
		"mail.cf.st.exists":     "Already exists",
		"mail.cf.st.conflict":   "Conflicts with existing record",
		"mail.cf.st.failed":     "Failed",
		"mail.cf.nohostip":      "No public IP detected; the A record for %s was not added. Add it manually.",
		"mail.cf.conflict":      "An existing record conflicts. The panel does not overwrite or delete it. Review and change it manually in Cloudflare.",
		"mail.cf.err.token":     "Invalid API token. Use 20–128 letters, numbers, underscores or hyphens.",
		"mail.cf.err.nodomain":  "This domain has not been added to the mail server.",
		"mail.cf.err.badtoken":  "Cloudflare rejected this API token. Check the token.",
		"mail.cf.err.noperm":    "Insufficient token permissions. Grant Zone · DNS · Edit and Zone · Zone · Read for the correct zone.",
		"mail.cf.err.nozone":    "The token cannot access a zone for this domain. Select the correct zone when creating it.",
		"mail.cf.err.ratelimit": "Cloudflare rate limit reached. Try again in a minute.",
		"mail.cf.err.network":   "Cannot connect to Cloudflare from this server.",
		"mail.cf.err.off":       "Enable the mail server first.",
	} {
		en[k] = v
	}
}

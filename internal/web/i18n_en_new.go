package web

// English text of the "add a website" flow.
func init() {
	for k, v := range map[string]string{
		"page.sitenew":     "Add website",
		"sites.new":        "Add website",
		"new.title":        "Add website",
		"new.sub":          "Choose the type of website to create.",
		"new.back":         "Back",
		"new.create":       "Create website",
		"new.desc.php":     "A PHP website such as WordPress or Laravel. Each website has its own PHP process pool.",
		"new.desc.python":  "A Python application (Flask, Django, FastAPI…) running as a service in its own virtual environment, with nginx forwarding requests to it.",
		"new.desc.node":    "A Node.js application running as a dedicated service, with nginx forwarding requests to it.",
		"new.desc.static":  "Serve existing HTML, CSS and image files.",
		"new.desc.proxy":   "Forward all requests to another address, such as an application on another server.",
		"f.hosts.hint":     "One domain, e.g. example.com. Configure www later in Overview and issue a certificate in the SSL tab.",
		"f.user":           "Website account",
		"f.user.hint":      "The main SSH/SFTP account for this website. Filled from the domain; you can change it.",
		"f.password":       "Account password",
		"f.password.hint":  "Used for SSH/SFTP. After creation, it appears only once on the result page: save it.",
		"f.password.gen":   "Generate new password",
		"f.password.empty": "Leave blank to generate automatically",
		"res.access":       "SSH/SFTP sign-in\n  server:   %s\n  username: %s\n  password: %s",
		"err.password":     "Invalid password: use 12–64 characters without whitespace, quotes, backslashes or backticks.",
		"err.sitename":     "Could not derive a website name from the domain. Check the domain.",
		"err.kind":         "Invalid website type.",
		"err.nohosts":      "Enter at least one domain.",
		"err.userreserved": "Account name %q is reserved by the system. Choose another.",
		"err.userexists":   "Account %q already exists on this server. Choose another.",
		"err.userused":     "Account %q is already used by website %q. Choose another.",
		"err.siteused":     "A website named %q already exists.",
		"err.phpmissing":   "PHP %q is not installed. Install with: mpn plan -php <version> -apply",
		"err.nodemissing":  "Node.js %q is not installed. Install with: mpn plan -node <version> -apply",
		"new.nophp":        "No PHP versions installed. Install with: mpn plan -php 8.4 -apply",
		"new.nonode":       "No Node.js versions installed. Install with: mpn plan -node 22 -apply",
		"err.exitstatus":   "The command failed. See the result details below.",
	} {
		en[k] = v
	}
}

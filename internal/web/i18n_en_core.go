package web

// English text of the shell of every page: the name, the navigation, the sign-in form and the errors of the first-run page.
func init() {
	for k, v := range map[string]string{
		"app.name":         "Mini Panel",
		"theme.toggle":     "Toggle light/dark theme",
		"nav.main":         "Main navigation",
		"nav.skip":         "Skip to content",
		"login.title":      "Sign in",
		"login.wrong":      "Incorrect username or password.",
		"login.locked":     "Too many failed attempts. Try again in %d seconds.",
		"page.login":       "Sign in",
		"page.setup":       "Set up Mini Panel",
		"page.notfound":    "Page not found",
		"setup.err.user":   "Invalid username. Use lowercase letters, numbers, hyphens or underscores, starting with a letter.",
		"setup.err.short":  "Password must contain at least %d characters.",
		"setup.err.long":   "Password is too long (maximum 256 characters).",
		"setup.err.match":  "Passwords do not match.",
		"setup.err.hash":   "Could not hash the password. Please try again.",
		"setup.err.nosave": "Cannot save the account: account storage has not been configured.",
		"setup.err.write":  "Could not save the account: %s",
	} {
		en[k] = v
	}
}

package web

// English text of the sign-in, first-run and not-found pages.
func init() {
	for k, v := range map[string]string{
		"notfound.description":     "This address does not exist or has changed. Check the URL or return to the home page.",
		"setup.description":        "This is the only account used to sign in to Mini Panel. Choose your username and password.",
		"setup.password.hint":      "At least 12 characters. You can use a passphrase.",
		"login.code.hint":          "Enter the 6-digit code from your authenticator app.",
		"login.description":        "Use your administrator account to continue.",
		"notfound.document.title":  "Page not found - Mini Panel",
		"setup.username.hint":      "lowercase letters, numbers and hyphens only",
		"setup.submit":             "Create account and sign in",
		"notfound.heading":         "This page could not be found",
		"login.document.title":     "Sign in - Mini Panel",
		"setup.heading":            "Create your account.",
		"foot.cpu":                 "%s CPU",
		"foot.ram":                 "%s RAM",
		"login.heading":            "Sign in to your panel.",
		"setup.document.title":     "Set up Mini Panel",
		"notfound.back":            "Go back",
		"setup.badge":              "INITIAL SETUP",
		"login.code.heading":       "Enter your verification code.",
		"setup.password.confirm":   "Confirm password",
		"notfound.home":            "Go to home page",
		"auth.username":            "Username",
		"auth.password.show.label": "Show password",
		"auth.skip":                "Skip to form",
		"auth.code":                "Verification code",
		"auth.signin":              "Sign in",
		"auth.confirm":             "Confirm",
		"auth.password":            "Password",
		"auth.password.show":       "Show",
		"auth.password.hide":       "Hide",
		"language.label":           "Language",
	} {
		en[k] = v
	}
}

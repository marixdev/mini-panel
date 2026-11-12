package web

// English text of the terminal.
func init() {
	for k, v := range map[string]string{
		"tab.terminal":    "Terminal",
		"term.hint":       "Signed in as the website account. The panel does not store what you type or see here.",
		"term.connecting": "Connecting…",
		"term.connected":  "Connected",
		"term.closed":     "Session ended",
		"term.failed":     "Connection failed",
		"term.off":        "Enable SSH/SFTP access for this website first in the tab",
		"term.loginfail":  "Cannot sign in as the website account. Check the panel key in the SSH/SFTP tab.",
		"term.toomany":    "Too many terminals open. Close some and try again.",
	} {
		en[k] = v
	}
}

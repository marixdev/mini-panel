package web

// English text of the certificate card of the security page: the certificate of the panel and of the database browser, which share one.
func init() {
	for k, v := range map[string]string{
		"psl.title":            "Panel SSL certificate",
		"psl.sub":              "The panel and phpMyAdmin share one certificate, removing browser warnings on both ports when accessed by domain.",
		"psl.mode.selfsigned":  "Self-signed",
		"psl.mode.letsencrypt": "Let's Encrypt",
		"psl.mode.custom":      "Custom certificate",
		"psl.selfsigned.d":     "The panel uses a self-signed certificate, causing browser warnings. Point a domain to this server and issue a certificate below.",
		"psl.domain":           "Domain",
		"psl.urls":             "Access addresses",
		"psl.panel":            "Panel",
		"psl.pma":              "phpMyAdmin",
		"psl.notcover":         "The current certificate does not cover this domain.",
		"psl.auto":             "Let's Encrypt, automatically renewed 30 days before expiration using the website renewal timer.",
		"psl.manual":           "Custom certificates do not renew automatically. Replace before expiration.",
		"psl.le.title":         "Issue with Let's Encrypt",
		"psl.le.desc":          "Free. The panel checks DNS, opens an nginx server block on port 80 for verification and installs the certificate for both ports.",
		"psl.le.btn":           "Issue certificate",
		"psl.le.replace":       "Reissue",
		"psl.le.hint":          "Point the domain's A record to %s and open port 80 to the Internet, including in your provider's firewall. Use a separate domain for the panel.",
		"psl.own.title":        "Use custom certificate",
		"psl.own.desc":         "Paste a purchased or externally issued certificate covering the domain above.",
		"psl.own.btn":          "Install certificate",
		"psl.relogin":          "After changing, open the panel using the new domain and sign in again. Sessions are tied to their original address.",
		"psl.off.title":        "Return to self-signed certificate",
		"psl.off.warn":         "The panel will stop responding on this domain. Access will remain available by IP with a self-signed certificate.",
		"psl.off.btn":          "Use self-signed certificate",
		"psl.fail":             "Action not completed",
		"psl.err.domain":       "Enter a lowercase domain containing a dot, e.g. panel.example.com. IP addresses are not supported.",
		"psl.err.nokey":        "Paste the certificate's private key.",
		"psl.err.site":         "This domain belongs to a website. Use a separate name for the panel, e.g. panel.example.com.",
		"psl.err.dns":          "The domain does not point to this server. Check the A record and retry in a few minutes.",
		"act.psl.le":           "Issue Let's Encrypt certificate for panel",
		"act.psl.custom":       "Install custom certificate for panel",
		"act.psl.off":          "Restore panel self-signed certificate",
	} {
		en[k] = v
	}
}

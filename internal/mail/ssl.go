package mail

import (
	"fmt"

	"mpn/internal/security"
)

const (
	// ACMEConf answers the challenge of Let's Encrypt on port 80 for the name of the mail host. It stays after the certificate is
	// issued, because the renewal needs it again.
	ACMEConf = "/etc/nginx/conf.d/05-mpn-mail-acme.conf"

	SettingSSL = "mail_ssl" // "selfsigned" (the default) or "letsencrypt"

	SSLSelfSigned  = "selfsigned"
	SSLLetsEncrypt = "letsencrypt"
)

// ACMEConfig is the nginx block for the mail host. Port 80 of this name serves nothing but the challenge: mail clients talk to
// Postfix and Dovecot, never to nginx.
func ACMEConfig(host string) (string, error) {
	if err := ValidDomain(host); err != nil {
		return "", fmt.Errorf("mail host: %w", err)
	}
	return "# Written by mpn: lets Let's Encrypt check the name of the mail host.\n" +
		"server {\n    listen 80;\n    listen [::]:80;\n    server_name " + host + ";\n    " + security.OffInclude + "\n" +
		"    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }\n" +
		"    location / { return 404; }\n}\n", nil
}

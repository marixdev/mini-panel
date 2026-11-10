package vhost

// The web mail under /webmail/ of a name. One program serves every name: nginx cuts the prefix before it passes a request on and names
// it in X-Forwarded-Prefix, and the program writes its links and cookies with that prefix. The program signs people in by their full
// address, so which name a person came by does not matter.
const (
	// WebmailBackendPort is where the program listens on this machine (see mail.WebmailBackendPort, which a test keeps equal).
	WebmailBackendPort = 8781
	WebmailPrefix      = "/webmail/"
)

// WebmailLocations is the part of a server block that serves the web mail. It belongs to blocks that serve over SSL: a request that
// reaches it over plain HTTP is sent to https first, so a password is never typed on a plain connection.
//
// The name shares an origin with the web mail, so a flaw in the site could reach the mailbox of a person who has it open. The headers
// narrow that: the page may only talk to /webmail/, may not be framed by another site, and is cut off from windows of the site.
// ModSecurity is left out because its rules would stop mail that contains code, which is ordinary in mail.
func WebmailLocations() string {
	return `    # The web mail under /webmail/ of this name.
    location = /webmail { return 301 /webmail/; }
    location ^~ /webmail/ {
        if ($scheme != https) { return 301 https://$host$request_uri; }
        include /etc/mpn/modsec/off.d/*.conf;
        proxy_pass http://127.0.0.1:8781/;
        proxy_http_version 1.1;
        # $http_host keeps the port; the program compares it with the Referer of a form.
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Prefix /webmail;
        proxy_buffering off;
        proxy_read_timeout 3600s;
        proxy_connect_timeout 60s;
        client_max_body_size 30m;
        add_header X-Content-Type-Options nosniff always;
        add_header Referrer-Policy same-origin always;
        add_header Cross-Origin-Opener-Policy same-origin always;
        add_header Content-Security-Policy "connect-src https://$http_host/webmail/; form-action https://$http_host/webmail/; frame-ancestors 'self'; base-uri 'self'" always;
    }
`
}

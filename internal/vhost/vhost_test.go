package vhost

import (
	"strings"
	"testing"

	"mpn/internal/site"
)

func phpSite() site.Site {
	return site.Site{Name: "blog", Kind: site.PHP, User: "blog", Root: "/home/blog/htdocs",
		Hostnames: []string{"blog.example.com", "www.blog.example.com"}, PHPVersion: "8.4"}
}

func mustContain(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
	if strings.Count(out, "{") != strings.Count(out, "}") {
		t.Errorf("unbalanced braces in:\n%s", out)
	}
}

func TestStaticWithoutTLS(t *testing.T) {
	s := phpSite()
	s.Kind, s.PHPVersion = site.Static, ""
	out, err := Render(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "server_name blog.example.com www.blog.example.com;", "try_files $uri $uri/ =404;")
	if strings.Count(out, "try_files $uri $uri/ =404;") != 1 || strings.Contains(out, "/etc/mpn/certs/blog") {
		t.Errorf("the site is served on port 80 only:\n%s", out)
	}
}

func TestPHPBBServesItsInstallerOnlyUntilItIsReady(t *testing.T) {
	s := site.Site{Name: "forum-mpanel-biz", Kind: site.PHP, User: "forum-mpanel", Hostnames: []string{"forum.mpanel.biz"}, Root: "/home/forum-mpanel/htdocs", PHPVersion: "8.4", App: "phpbb"}
	out, err := Render(s, Options{TLS: true, CertDir: "/etc/mpn/certs/forum-mpanel-biz"})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "location ^~ /install/ {", "rewrite ^(.*)$ /install/app.php/$1 last;", "fastcgi_param PATH_INFO $fastcgi_path_info;",
		"location @rewriteapp { rewrite ^(.*)$ /index.php/$1 last; }", "images/avatars/upload", "fastcgi_pass unix:/run/php/forum-mpanel-biz.sock;", "location /.well-known/acme-challenge/")
	s.AppReady = true
	out, err = Render(s, Options{TLS: true, CertDir: "/etc/mpn/certs/forum-mpanel-biz"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "/install/") {
		t.Errorf("a ready phpBB must not serve its installer:\n%s", out)
	}
	mustContain(t, out, "location @rewriteapp", "deny all;")
}

func TestPHPWithTLS(t *testing.T) {
	out, err := Render(phpSite(), Options{TLS: true, CertDir: "/etc/mpn/certs/blog"})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + out)
	mustContain(t, out, "listen 443 ssl http2;", "return 301 https://$host$request_uri;",
		"ssl_certificate /etc/mpn/certs/blog/fullchain.pem;", "fastcgi_pass unix:/run/php/blog.sock;",
		"/index.php?$query_string;")
}

// A site without a certificate still answers https:// for its names, so nginx does not hand the visitor to the catch-all
// page; it sends them to http://, where the site is.
func TestSiteWithoutTLSAnswersHTTPSByRedirect(t *testing.T) {
	s := phpSite()
	s.WWW = site.WWWBare
	out, err := Render(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "listen 443 ssl http2;", "ssl_certificate "+DefaultCertDir+"/fullchain.pem;",
		"server_name blog.example.com www.blog.example.com;\n    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }\n    location / { return 302 http://$host$request_uri; }")
	if strings.Count(out, "listen 443") != 1 {
		t.Errorf("one 443 server expected:\n%s", out)
	}
	tls, _ := Render(phpSite(), Options{TLS: true, CertDir: "/etc/mpn/certs/blog"})
	if strings.Contains(tls, DefaultCertDir) || strings.Contains(tls, "return 302") {
		t.Errorf("a site with its certificate must not use the default one:\n%s", tls)
	}
}

func TestNodeAndProxy(t *testing.T) {
	n := phpSite()
	n.Kind, n.PHPVersion, n.NodePort, n.NodeVersion, n.Entry = site.Node, "", 3001, "22", "server.js"
	out, err := Render(n, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "proxy_pass http://127.0.0.1:3001;", "proxy_set_header Connection $connection_upgrade;")
	p := phpSite()
	p.Kind, p.PHPVersion, p.Upstream = site.Proxy, "", "https://10.0.0.5:8443"
	out, err = Render(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "proxy_pass https://10.0.0.5:8443;")
}

func TestRejectsUnsafeInput(t *testing.T) {
	bad := phpSite()
	bad.Hostnames = []string{"a.com; } server {"}
	if _, err := Render(bad, Options{}); err == nil {
		t.Error("host injection must be rejected")
	}
	if _, err := Render(phpSite(), Options{TLS: true, CertDir: "/etc/x; y"}); err == nil {
		t.Error("unsafe cert dir must be rejected")
	}
}

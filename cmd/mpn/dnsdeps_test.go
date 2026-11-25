package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// The check follows a redirect to https on the same name, as Let's Encrypt does (Cloudflare's "Always use HTTPS"), and a relative
// one, but not a redirect to another name: the file must come from the name that is checked.
func TestTheProbeFollowsRedirectsOnTheSameNameOnly(t *testing.T) {
	const name = "blog.example.com"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != name {
			http.Error(w, "wrong host "+r.Host, 400)
			return
		}
		switch r.URL.Path {
		case "/rel":
			http.Redirect(w, r, "/ok", http.StatusMovedPermanently)
		case "/other":
			http.Redirect(w, r, "http://elsewhere.example.net/ok", http.StatusMovedPermanently)
		case "/ok":
			_, _ = w.Write([]byte("token"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	get := func(path string) (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Host = name
		resp, err := probeClient(name).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Request.URL.Host
	}
	u, _ := url.Parse(srv.URL)
	if code, host := get("/rel"); code != 200 || host != u.Host {
		t.Errorf("a relative redirect is followed with the name kept: %d %s", code, host)
	}
	if code, _ := get("/other"); code != http.StatusMovedPermanently {
		t.Errorf("a redirect to another name is not followed: %d", code)
	}
}

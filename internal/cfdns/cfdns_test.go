package cfdns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	tok = "tok_0123456789abcdefghij"
	zid = "0123456789abcdef0123456789abcdef"
)

// fake stands in for the API: only the calls the package makes, and it counts any call that would change or delete a record.
type fake struct {
	mu      sync.Mutex
	zones   map[string]string
	recs    []Record
	n       int
	posts   []createBody
	bad     int // requests with a method the package must never use
	hits    int
	auths   []string
	mode    string // "", "forbid", "rate", "dup", "big", "badpost"
	echo    bool   // error messages repeat the Authorization header, as a careless server might
	verify  string
	deleted []string
}

func (f *fake) reply(w http.ResponseWriter, status int, ok bool, result any, code int, msg string) {
	body := map[string]any{"success": ok, "result": result, "result_info": map[string]int{"total_pages": 1}, "errors": []any{}}
	if code != 0 {
		body["errors"] = []map[string]any{{"code": code, "message": msg}}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++
	a := r.Header.Get("Authorization")
	f.auths = append(f.auths, a)
	echo := func(s string) string {
		if f.echo {
			return s + " [" + a + "]"
		}
		return s
	}
	switch f.mode {
	case "rate":
		f.reply(w, 429, false, nil, 971, "rate limited")
		return
	case "big":
		_, _ = w.Write(bytes.Repeat([]byte("x"), 2<<20))
		return
	}
	if a != "Bearer "+tok {
		f.reply(w, 401, false, nil, 1000, echo("Invalid API Token"))
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/client/v4")
	switch {
	case r.Method == "GET" && p == "/user/tokens/verify":
		st := f.verify
		if st == "" {
			st = "active"
		}
		f.reply(w, 200, true, map[string]string{"id": "t1", "status": st}, 0, "")
	case r.Method == "GET" && p == "/zones":
		if f.mode == "forbid" {
			f.reply(w, 403, false, nil, 10000, echo("Authentication error"))
			return
		}
		name := r.URL.Query().Get("name")
		out := []Zone{}
		if id, ok := f.zones[name]; ok {
			out = append(out, Zone{ID: id, Name: name})
		}
		f.reply(w, 200, true, out, 0, "")
	case strings.HasPrefix(p, "/zones/"+zid+"/dns_records"):
		if f.mode == "forbid" {
			f.reply(w, 403, false, nil, 10000, echo("Authentication error"))
			return
		}
		switch r.Method {
		case "GET":
			out := []Record{}
			for _, rec := range f.recs {
				if n := r.URL.Query().Get("name"); n != "" && rec.Name != n {
					continue
				}
				if t := r.URL.Query().Get("type"); t != "" && rec.Type != t {
					continue
				}
				out = append(out, rec)
			}
			f.reply(w, 200, true, out, 0, "")
		case "POST":
			var b createBody
			_ = json.NewDecoder(r.Body).Decode(&b)
			switch f.mode {
			case "dup":
				f.reply(w, 400, false, nil, 81058, "An identical record already exists.")
				return
			case "badpost":
				f.reply(w, 400, false, nil, 9999, echo("invalid content"))
				return
			}
			f.n++
			rec := Record{ID: fmt.Sprintf("r%d", f.n), Type: b.Type, Name: b.Name, Content: b.Content, Priority: b.Priority, TTL: b.TTL, Proxied: b.Proxied != nil && *b.Proxied}
			f.recs = append(f.recs, rec)
			f.posts = append(f.posts, b)
			f.reply(w, 200, true, rec, 0, "")
		case "DELETE":
			// Only a record this fake made, by its exact address: the challenge record the package added itself.
			id := strings.TrimPrefix(p, "/zones/"+zid+"/dns_records/")
			for i, rec := range f.recs {
				if rec.ID == id && strings.HasPrefix(rec.Name, "_acme-challenge.") {
					f.recs = append(f.recs[:i], f.recs[i+1:]...)
					f.deleted = append(f.deleted, id)
					f.reply(w, 200, true, map[string]string{"id": id}, 0, "")
					return
				}
			}
			f.bad++
			http.Error(w, "no", 404)
		default:
			f.bad++
			http.Error(w, "no", 405)
		}
	default:
		f.bad++
		http.NotFound(w, r)
	}
}

func newFake(t *testing.T) (*fake, Client, *httptest.Server) {
	t.Helper()
	f := &fake{zones: map[string]string{"example.com": zid}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, Client{Token: tok, BaseURL: srv.URL + "/client/v4"}, srv
}

// ---- detection ----

type nsRes struct {
	m     map[string][]string
	err   map[string]error
	asked []string
}

func (r *nsRes) LookupNS(_ context.Context, name string) ([]*net.NS, error) {
	r.asked = append(r.asked, name)
	if e, ok := r.err[name]; ok {
		return nil, e
	}
	hosts, ok := r.m[name]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	var out []*net.NS
	for _, h := range hosts {
		out = append(out, &net.NS{Host: h})
	}
	return out, nil
}

func TestDetect(t *testing.T) {
	cf := []string{"bob.ns.cloudflare.com.", "Kay.NS.Cloudflare.com."}
	r := &nsRes{m: map[string][]string{
		"example.com":     cf,
		"sub.example.com": {"ns1.other.net."},
		"example.org":     {"ns1.googledomains.com."},
		"fake.net":        {"x.ns.cloudflare.com.evil.net.", "fakens.cloudflare.com.", "ns.cloudflare.com."},
	}}
	for _, c := range []struct {
		domain string
		want   Detection
	}{
		{"example.com", Detection{true, "example.com"}},
		{"Mail.Example.COM.", Detection{true, "example.com"}}, // the subdomain does not exist, so the parent decides
		{"www.sub.example.com", Detection{}},                  // a subdomain with name servers of its own is not served by the parent's
		{"example.org", Detection{}},
		{"fake.net", Detection{}}, // the look-alikes are not Cloudflare
		{"nothing.test", Detection{}},
	} {
		got, err := Detect(context.Background(), r, c.domain)
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.domain, got, err, c.want)
		}
	}
}

func TestDetectStopsAtTwoLabelsAndSaysWhenItCouldNotTell(t *testing.T) {
	r := &nsRes{m: map[string][]string{}}
	if _, err := Detect(context.Background(), r, "a.b.example.com"); err != nil {
		t.Fatal(err)
	}
	for _, name := range r.asked {
		if name == "com" {
			t.Error("a top-level name is never asked")
		}
	}
	r = &nsRes{}
	if d, err := Detect(context.Background(), r, "localhost"); err != nil || d.Cloudflare || len(r.asked) != 0 {
		t.Errorf("a single label: %+v %v %v", d, err, r.asked)
	}
	r = &nsRes{m: map[string][]string{"example.com": {"a.ns.cloudflare.com."}}, err: map[string]error{"mail.example.com": &net.DNSError{Err: "i/o timeout", IsTimeout: true, IsTemporary: true}}}
	if d, err := Detect(context.Background(), r, "mail.example.com"); err == nil || d.Cloudflare {
		t.Errorf("a timeout must be an error and not a quiet 'no': %+v %v", d, err)
	}
	if _, err := Detect(context.Background(), r, " "); err == nil {
		t.Error("no domain is an error")
	}
}

// ---- the client ----

func TestVerifyTokenSendsTheBearerHeaderAndReadsTheStatus(t *testing.T) {
	f, c, _ := newFake(t)
	if err := c.VerifyToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.auths[0] != "Bearer "+tok {
		t.Errorf("header %q", f.auths[0])
	}
	f.verify = "expired"
	if err := c.VerifyToken(context.Background()); !errors.Is(err, ErrBadToken) {
		t.Errorf("an inactive token: %v", err)
	}
	wrong := c
	wrong.Token = "wrong_0123456789abcdefgh"
	if err := wrong.VerifyToken(context.Background()); !errors.Is(err, ErrBadToken) {
		t.Errorf("a wrong token: %v", err)
	}
}

func TestAMalformedTokenIsRefusedBeforeAnythingIsSent(t *testing.T) {
	f, c, _ := newFake(t)
	for _, bad := range []string{"", "short", "has space 0123456789abcdef", "line\nbreak0123456789abcdef", "tok_0123456789abcdefghij\r\nX-Evil: 1"} {
		c.Token = bad
		if err := c.VerifyToken(context.Background()); !errors.Is(err, ErrBadToken) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if f.hits != 0 {
		t.Errorf("%d requests were made with a token that cannot be one", f.hits)
	}
}

func TestFindZoneWalksUpToTheParent(t *testing.T) {
	f, c, _ := newFake(t)
	z, err := c.FindZone(context.Background(), "Mail.Sub.Example.com.")
	if err != nil || z.ID != zid || z.Name != "example.com" {
		t.Errorf("%+v %v", z, err)
	}
	if _, err := c.FindZone(context.Background(), "other.org"); !errors.Is(err, ErrNoZone) {
		t.Errorf("a domain that is not in the account: %v", err)
	}
	f.mode = "forbid"
	if _, err := c.FindZone(context.Background(), "example.com"); !errors.Is(err, ErrNoPermission) {
		t.Errorf("a token without Zone:Read: %v", err)
	}
}

func TestTheRateLimitHasItsOwnError(t *testing.T) {
	f, c, _ := newFake(t)
	f.mode = "rate"
	if err := c.VerifyToken(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Errorf("%v", err)
	}
	if _, err := c.ListRecords(context.Background(), zid, "", "example.com"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("%v", err)
	}
}

func TestTheTokenNeverShowsInAnErrorOrWhenAClientIsPrinted(t *testing.T) {
	f, c, _ := newFake(t)
	f.echo = true
	var errs []error
	wrong := c
	wrong.Token = "wrong_0123456789abcdefgh"
	errs = append(errs, wrong.VerifyToken(context.Background()))
	f.mode = "forbid"
	_, err := c.FindZone(context.Background(), "example.com")
	errs = append(errs, err)
	_, err = c.ListRecords(context.Background(), zid, "", "example.com")
	errs = append(errs, err)
	f.mode = "badpost"
	_, err = c.CreateRecord(context.Background(), zid, Record{Type: "TXT", Name: "example.com", Content: "x"})
	errs = append(errs, err)
	for i, e := range errs {
		if e == nil {
			t.Errorf("%d: no error", i)
			continue
		}
		if strings.Contains(e.Error(), tok) || strings.Contains(e.Error(), "wrong_0123456789") {
			t.Errorf("%d: the token is in the error: %v", i, e)
		}
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if s := fmt.Sprintf(verb, c); strings.Contains(s, tok) {
			t.Errorf("%s prints the token: %s", verb, s)
		}
	}
	if s := fmt.Sprintf("%v", &c); strings.Contains(s, tok) {
		t.Errorf("a pointer prints the token: %s", s)
	}
}

func TestARedirectToAnotherHostIsRefusedAndTheTokenStaysHome(t *testing.T) {
	var seen []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer srv.Close()
	c := Client{Token: tok, BaseURL: srv.URL + "/client/v4"}
	if err := c.VerifyToken(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Errorf("%v", err)
	}
	if len(seen) != 0 {
		t.Errorf("the other host was reached with %v", seen)
	}
}

func TestAnAnswerOverOneMegabyteIsRefused(t *testing.T) {
	f, c, _ := newFake(t)
	f.mode = "big"
	if err := c.VerifyToken(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Errorf("%v", err)
	}
}

func TestTheAPIAddressMustBeHTTPSExceptOnThisMachine(t *testing.T) {
	for _, base := range []string{"http://api.example.com/client/v4", "ftp://x/y", "https://user:pw@api.example.com/", "::bad"} {
		err := Client{Token: tok, BaseURL: base}.VerifyToken(context.Background())
		if !errors.Is(err, ErrNetwork) {
			t.Errorf("%s: %v", base, err)
		}
	}
}

func TestAServerThatIsGoneIsANetworkError(t *testing.T) {
	_, c, srv := newFake(t)
	srv.Close()
	err := c.VerifyToken(context.Background())
	if !errors.Is(err, ErrNetwork) || strings.Contains(err.Error(), tok) {
		t.Errorf("%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.VerifyToken(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled call says so: %v", err)
	}
}

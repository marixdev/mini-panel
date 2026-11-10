package cfdns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the Cloudflare API.
	DefaultBaseURL = "https://api.cloudflare.com/client/v4"
	maxBody        = 1 << 20 // the most of an answer that is read
	maxPages       = 20
)

// Client talks to the API with a token from "My Profile > API Tokens" (Zone:Read and DNS:Edit on the zone). Nothing about it is kept.
type Client struct {
	Token   string
	BaseURL string       // DefaultBaseURL when empty; http is accepted only for a loopback address (tests)
	HTTP    *http.Client // a client with a 20 second timeout when nil
}

// String and GoString keep the token out of anything that prints a Client.
func (c Client) String() string   { return "cfdns.Client{Token:***}" }
func (c Client) GoString() string { return c.String() }

// APIError is an answer or a failure of the API. Kind (one of the Err values above) is what errors.Is finds.
type APIError struct {
	Status  int
	Codes   []int
	Message string
	Kind    error
}

func (e *APIError) Error() string {
	if e.Kind != nil {
		if e.Message == "" {
			return e.Kind.Error()
		}
		return e.Kind.Error() + " (" + e.Message + ")"
	}
	return fmt.Sprintf("cloudflare: HTTP %d: %s", e.Status, e.Message)
}

func (e *APIError) Unwrap() error { return e.Kind }

func (e *APIError) has(code int) bool {
	for _, c := range e.Codes {
		if c == code {
			return true
		}
	}
	return false
}

var (
	tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{20,128}$`) // what a token looks like; also keeps a line break out of a header
	idRe    = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
)

func (c Client) scrub(s string) string {
	if c.Token != "" {
		s = strings.ReplaceAll(s, c.Token, "[token]")
	}
	return s
}

func (c Client) base() (*url.URL, error) {
	b := c.BaseURL
	if b == "" {
		b = DefaultBaseURL
	}
	u, err := url.Parse(b)
	if err != nil || u.Host == "" || u.User != nil {
		return nil, &APIError{Kind: ErrNetwork, Message: "the API address is not valid"}
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return nil, &APIError{Kind: ErrNetwork, Message: "the API address must be https"}
		}
	}
	return u, nil
}

// sameHost refuses a redirect to another host or from https to http: the token travels in a header, and must reach the API and nobody else.
func sameHost(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	first := via[0].URL
	if req.URL.Host != first.Host || (first.Scheme == "https" && req.URL.Scheme != "https") {
		return errors.New("a redirect to another address was refused")
	}
	return nil
}

func (c Client) httpClient() *http.Client {
	var hc http.Client
	if c.HTTP != nil {
		hc = *c.HTTP
	} else {
		hc.Timeout = 20 * time.Second
	}
	hc.CheckRedirect = sameHost
	return &hc
}

type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

// call makes one request and turns every way it can go wrong into an *APIError.
func (c Client) call(ctx context.Context, method, path string, q url.Values, body any) (envelope, error) {
	if !tokenRe.MatchString(c.Token) {
		return envelope{}, &APIError{Kind: ErrBadToken, Message: "the token is empty or has characters a token never has"}
	}
	base, err := c.base()
	if err != nil {
		return envelope{}, err
	}
	u := *base
	u.Path = strings.TrimSuffix(base.Path, "/") + path
	u.RawQuery = q.Encode()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return envelope{}, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return envelope{}, &APIError{Kind: ErrNetwork, Message: c.scrub(err.Error())}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return envelope{}, ctx.Err()
		}
		return envelope{}, &APIError{Kind: ErrNetwork, Message: c.scrub(err.Error())}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return envelope{}, &APIError{Kind: ErrNetwork, Message: c.scrub(err.Error())}
	}
	if len(raw) > maxBody {
		return envelope{}, &APIError{Kind: ErrNetwork, Message: "the answer is larger than 1 MB"}
	}
	var env envelope
	jerr := json.Unmarshal(raw, &env)
	var msgs []string
	var codes []int
	for _, e := range env.Errors {
		codes = append(codes, e.Code)
		msgs = append(msgs, e.Message)
	}
	msg := c.scrub(strings.Join(msgs, "; "))
	has := func(want ...int) bool {
		for _, c := range codes {
			for _, w := range want {
				if c == w {
					return true
				}
			}
		}
		return false
	}
	var kind error
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		kind = ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized:
		kind = ErrBadToken
	case resp.StatusCode == http.StatusForbidden:
		kind = ErrNoPermission
	case resp.StatusCode == http.StatusBadRequest && has(6003, 6111, 9106, 9109): // a token that is not shaped like one
		kind = ErrBadToken
	case resp.StatusCode >= 500:
		kind = ErrNetwork
	case resp.StatusCode/100 != 2:
	case jerr != nil:
		kind, msg = ErrNetwork, "the answer is not JSON"
	case !env.Success:
	default:
		return env, nil
	}
	return envelope{}, &APIError{Status: resp.StatusCode, Codes: codes, Message: msg, Kind: kind}
}

// VerifyToken asks whether the token is active. A token made under an account (not under a person) is verified at another address and
// answers 401 here, so a caller may carry on to FindZone when only this step says the token is bad.
func (c Client) VerifyToken(ctx context.Context) error {
	env, err := c.call(ctx, http.MethodGet, "/user/tokens/verify", nil, nil)
	if err != nil {
		return err
	}
	var r struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(env.Result, &r) != nil || r.Status != "active" {
		return &APIError{Kind: ErrBadToken, Message: "the token is not active"}
	}
	return nil
}

// Zone is a zone of the account.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FindZone looks for the zone of domain: the domain itself, then each parent with at least two labels. Listing zones needs Zone:Read.
func (c Client) FindZone(ctx context.Context, domain string) (Zone, error) {
	name := cleanName(domain)
	labels := strings.Split(name, ".")
	for i := 0; name != "" && len(labels)-i >= 2; i++ {
		cand := strings.Join(labels[i:], ".")
		env, err := c.call(ctx, http.MethodGet, "/zones", url.Values{"name": {cand}}, nil)
		if err != nil {
			return Zone{}, err
		}
		var zs []Zone
		if json.Unmarshal(env.Result, &zs) != nil {
			return Zone{}, &APIError{Kind: ErrNetwork, Message: "the list of zones is not what was expected"}
		}
		for _, z := range zs {
			if cleanName(z.Name) == cand && idRe.MatchString(z.ID) {
				return z, nil
			}
		}
	}
	return Zone{}, fmt.Errorf("%w: %s", ErrNoZone, name)
}

// Record is a DNS record. TTL 1 means automatic. ID is set on records read from the zone.
type Record struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Priority *int   `json:"priority,omitempty"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
}

// ListRecords returns the records of the zone, optionally only of one type and one exact name.
func (c Client) ListRecords(ctx context.Context, zoneID, typ, name string) ([]Record, error) {
	if !idRe.MatchString(zoneID) {
		return nil, &APIError{Kind: ErrNoZone, Message: "the zone id is not valid"}
	}
	var out []Record
	for page := 1; page <= maxPages; page++ {
		q := url.Values{"per_page": {"100"}, "page": {fmt.Sprint(page)}}
		if typ != "" {
			q.Set("type", strings.ToUpper(typ))
		}
		if name != "" {
			q.Set("name", cleanName(name))
		}
		env, err := c.call(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records", q, nil)
		if err != nil {
			return nil, err
		}
		var rs []Record
		if json.Unmarshal(env.Result, &rs) != nil {
			return nil, &APIError{Kind: ErrNetwork, Message: "the list of records is not what was expected"}
		}
		out = append(out, rs...)
		if page >= env.ResultInfo.TotalPages {
			break
		}
	}
	return out, nil
}

type createBody struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Priority *int   `json:"priority,omitempty"`
	TTL      int    `json:"ttl"`
	Proxied  *bool  `json:"proxied,omitempty"` // only A, AAAA and CNAME can be proxied
}

// CreateRecord adds one record. A TXT value longer than 255 characters (a 2048-bit DKIM key is about 400) is sent as one string. Whether
// Cloudflare splits it into 255-character pieces itself, as its dashboard does, has not been checked against the real API.
func (c Client) CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	if !idRe.MatchString(zoneID) {
		return Record{}, &APIError{Kind: ErrNoZone, Message: "the zone id is not valid"}
	}
	if r.TTL == 0 {
		r.TTL = 1
	}
	b := createBody{Type: strings.ToUpper(r.Type), Name: cleanName(r.Name), Content: r.Content, Priority: r.Priority, TTL: r.TTL}
	if b.Type == "A" || b.Type == "AAAA" || b.Type == "CNAME" {
		p := r.Proxied
		b.Proxied = &p
	}
	env, err := c.call(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", nil, b)
	if err != nil {
		return Record{}, err
	}
	var made Record
	if json.Unmarshal(env.Result, &made) != nil || made.ID == "" {
		made = r
	}
	return made, nil
}

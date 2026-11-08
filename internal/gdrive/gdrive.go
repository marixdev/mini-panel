// Package gdrive is a small client of Google Drive that uses only net/http: the OAuth device flow, a refreshed access token, folders,
// listing, deleting, and a resumable upload that goes on after a failure. It asks for the scope drive.file, which lets the program
// see only the files it made itself.
package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Scope      = "https://www.googleapis.com/auth/drive.file"
	FolderMime = "application/vnd.google-apps.folder"
)

// chunk is the size of one piece of an upload. Google wants a multiple of 256 KiB; 8 MiB is small enough for a 1 GB server.
// A test makes it smaller.
var chunk = 8 << 20

// Endpoints are the addresses of Google. A test points them at a server of its own.
type Endpoints struct{ DeviceCode, Token, API, Upload string }

var Google = Endpoints{
	DeviceCode: "https://oauth2.googleapis.com/device/code",
	Token:      "https://oauth2.googleapis.com/token",
	API:        "https://www.googleapis.com/drive/v3",
	Upload:     "https://www.googleapis.com/upload/drive/v3/files",
}

var (
	ErrPending  = errors.New("the person has not allowed access yet")
	ErrSlowDown = errors.New("asking too often")
	ErrDenied   = errors.New("access was denied")
	ErrExpired  = errors.New("the code has expired; start again")
	ErrRevoked  = errors.New("Google no longer accepts the saved permission (invalid_grant); connect again")
)

// APIError is an answer of Google that says no.
type APIError struct {
	Status          int
	Reason, Message string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("Google Drive: %s (%s, HTTP %d)", e.Message, e.Reason, e.Status)
	}
	return fmt.Sprintf("Google Drive: HTTP %d %s", e.Status, e.Message)
}

func apiError(status int, body []byte) error {
	var r struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	e := &APIError{Status: status, Message: strings.TrimSpace(string(body))}
	if len(e.Message) > 200 {
		e.Message = e.Message[:200]
	}
	if json.Unmarshal(body, &r) == nil && r.Error.Message != "" {
		e.Message = r.Error.Message
		if len(r.Error.Errors) > 0 {
			e.Reason = r.Error.Errors[0].Reason
		}
	}
	return e
}

func postForm(ctx context.Context, hc *http.Client, endpoint string, v url.Values) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(v.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, b, err
}

type tokenReply struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func oauthError(t tokenReply) error {
	switch t.Error {
	case "authorization_pending":
		return ErrPending
	case "slow_down":
		return ErrSlowDown
	case "access_denied":
		return ErrDenied
	case "expired_token":
		return ErrExpired
	case "invalid_grant":
		return ErrRevoked
	}
	return fmt.Errorf("Google refused the request: %s %s", t.Error, t.Description)
}

// Device is what the person needs to allow access: the code to type at the address, and for how long it is good.
type Device struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Error           string `json:"error"`
	Description     string `json:"error_description"`
}

// StartDevice asks Google for a code to show to the person.
func StartDevice(ctx context.Context, hc *http.Client, ep Endpoints, clientID string) (Device, error) {
	st, body, err := postForm(ctx, hc, ep.DeviceCode, url.Values{"client_id": {clientID}, "scope": {Scope}})
	if err != nil {
		return Device{}, err
	}
	var d Device
	if json.Unmarshal(body, &d) != nil || st != 200 || d.DeviceCode == "" {
		if d.Error != "" {
			return Device{}, fmt.Errorf("Google refused the client: %s %s", d.Error, d.Description)
		}
		return Device{}, fmt.Errorf("Google answered HTTP %d to the request for a code", st)
	}
	if d.Interval < 1 {
		d.Interval = 5
	}
	return d, nil
}

// PollDevice asks whether the person has allowed access. It returns the refresh token when they have, ErrPending while they have
// not, and ErrSlowDown, ErrDenied or ErrExpired otherwise.
func PollDevice(ctx context.Context, hc *http.Client, ep Endpoints, clientID, secret, deviceCode string) (string, error) {
	_, body, err := postForm(ctx, hc, ep.Token, url.Values{"client_id": {clientID}, "client_secret": {secret}, "device_code": {deviceCode},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}})
	if err != nil {
		return "", err
	}
	var t tokenReply
	if err := json.Unmarshal(body, &t); err != nil {
		return "", fmt.Errorf("Google gave an answer that is not JSON")
	}
	if t.Error != "" {
		return "", oauthError(t)
	}
	if t.RefreshToken == "" {
		return "", errors.New("Google gave no refresh token")
	}
	return t.RefreshToken, nil
}

// Client works with one Drive, as the person who allowed it.
type Client struct {
	HTTP                                 *http.Client
	EP                                   Endpoints
	ClientID, ClientSecret, RefreshToken string
	// Sleep and Now exist for tests; time.Sleep and time.Now when nil.
	Sleep func(time.Duration)
	Now   func() time.Time

	mu     sync.Mutex
	access string
	expiry time.Time
}

func (c *Client) hc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (c *Client) sleep(d time.Duration) {
	if c.Sleep != nil {
		c.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) token(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.access != "" && c.now().Before(c.expiry) {
		return c.access, nil
	}
	_, body, err := postForm(ctx, c.hc(), c.EP.Token, url.Values{"grant_type": {"refresh_token"}, "client_id": {c.ClientID},
		"client_secret": {c.ClientSecret}, "refresh_token": {c.RefreshToken}})
	if err != nil {
		return "", err
	}
	var t tokenReply
	if json.Unmarshal(body, &t) != nil {
		return "", errors.New("Google gave an answer that is not JSON")
	}
	if t.Error != "" {
		return "", oauthError(t)
	}
	c.access, c.expiry = t.AccessToken, c.now().Add(time.Duration(t.ExpiresIn-60)*time.Second)
	return c.access, nil
}

// once sends one request. With auth it adds the access token and, when Google says the token is no good, gets a new one and sends again.
func (c *Client) once(ctx context.Context, method, u string, h http.Header, body []byte, auth bool) (int, http.Header, []byte, error) {
	for try := 0; ; try++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
		if err != nil {
			return 0, nil, nil, err
		}
		for k, v := range h {
			req.Header[k] = v
		}
		if auth {
			tok, err := c.token(ctx, try > 0)
			if err != nil {
				return 0, nil, nil, err
			}
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := c.hc().Do(req)
		if err != nil {
			return 0, nil, nil, err
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return 0, nil, nil, err
		}
		if resp.StatusCode == 401 && auth && try == 0 {
			continue
		}
		return resp.StatusCode, resp.Header, b, nil
	}
}

func backoff(attempt int) time.Duration { return time.Second << min(attempt, 5) }

// send is once, repeated when the network fails or Google says 429 or 5xx. A 4xx answer is returned as an error at once.
func (c *Client) send(ctx context.Context, method, u string, h http.Header, body []byte) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		st, _, b, err := c.once(ctx, method, u, h, body, true)
		switch {
		case err == nil && st < 300:
			return b, nil
		case err == nil && st != 429 && st < 500:
			return nil, apiError(st, b)
		case err == nil:
			last = apiError(st, b)
		default:
			last = err
		}
		if ctx.Err() != nil || errors.Is(last, ErrRevoked) {
			return nil, last
		}
		c.sleep(backoff(attempt))
	}
	return nil, last
}

func qs(s string) string { return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) }

// File is a file or a folder of Drive.
type File struct {
	ID, Name, MimeType string
	MD5                string // of the content, as Google computed it; empty for a folder
	Size               int64
}

func (f File) IsFolder() bool { return f.MimeType == FolderMime }

// List returns what is directly inside a folder.
func (c *Client) List(ctx context.Context, parent string) ([]File, error) {
	var out []File
	for page := ""; ; {
		q := url.Values{"q": {"'" + qs(parent) + "' in parents and trashed=false"}, "fields": {"nextPageToken,files(id,name,mimeType,size,md5Checksum)"}, "pageSize": {"200"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		b, err := c.send(ctx, "GET", c.EP.API+"/files?"+q.Encode(), nil, nil)
		if err != nil {
			return nil, err
		}
		var r struct {
			Next  string `json:"nextPageToken"`
			Files []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Mime string `json:"mimeType"`
				Size string `json:"size"`
				MD5  string `json:"md5Checksum"`
			} `json:"files"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, errors.New("Google Drive gave an answer that is not JSON")
		}
		for _, f := range r.Files {
			n, _ := strconv.ParseInt(f.Size, 10, 64)
			out = append(out, File{ID: f.ID, Name: f.Name, MimeType: f.Mime, MD5: f.MD5, Size: n})
		}
		if r.Next == "" {
			return out, nil
		}
		page = r.Next
	}
}

// EnsureFolder returns the folder of that name inside parent, making it when it is not there.
func (c *Client) EnsureFolder(ctx context.Context, parent, name string) (string, error) {
	q := url.Values{"q": {"mimeType='" + FolderMime + "' and name='" + qs(name) + "' and '" + qs(parent) + "' in parents and trashed=false"}, "fields": {"files(id)"}}
	b, err := c.send(ctx, "GET", c.EP.API+"/files?"+q.Encode(), nil, nil)
	if err != nil {
		return "", err
	}
	var r struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if json.Unmarshal(b, &r) == nil && len(r.Files) > 0 {
		return r.Files[0].ID, nil
	}
	meta, _ := json.Marshal(map[string]any{"name": name, "mimeType": FolderMime, "parents": []string{parent}})
	b, err = c.send(ctx, "POST", c.EP.API+"/files?fields=id", http.Header{"Content-Type": {"application/json"}}, meta)
	if err != nil {
		return "", err
	}
	var made struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(b, &made) != nil || made.ID == "" {
		return "", errors.New("Google Drive made no folder")
	}
	return made.ID, nil
}

// Delete removes a file or a folder with everything in it. It does not go to the trash.
func (c *Client) Delete(ctx context.Context, id string) error {
	_, err := c.send(ctx, "DELETE", c.EP.API+"/files/"+url.PathEscape(id), nil, nil)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == 404 {
		return nil
	}
	return err
}

// Email returns the address of the account, or "" when Google does not say.
func (c *Client) Email(ctx context.Context) string {
	b, err := c.send(ctx, "GET", c.EP.API+"/about?fields=user(emailAddress)", nil, nil)
	if err != nil {
		return ""
	}
	var r struct {
		User struct {
			Email string `json:"emailAddress"`
		} `json:"user"`
	}
	_ = json.Unmarshal(b, &r)
	return r.User.Email
}

// Upload sends a file into a folder in pieces and returns its id. When a piece fails the upload asks Google how much arrived and goes
// on from there, so a cut connection costs one piece, not the whole file.
func (c *Client) Upload(ctx context.Context, parent, name, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := fi.Size()
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{parent}})
	h := http.Header{"Content-Type": {"application/json; charset=UTF-8"}, "X-Upload-Content-Length": {strconv.FormatInt(size, 10)}, "X-Upload-Content-Type": {"application/octet-stream"}}
	var session string
	var last error
	for attempt := 0; attempt < 5 && session == ""; attempt++ {
		st, hdr, b, err := c.once(ctx, "POST", c.EP.Upload+"?uploadType=resumable&fields=id", h, meta, true)
		switch {
		case err == nil && st == 200 && hdr.Get("Location") != "":
			session = hdr.Get("Location")
		case err == nil && st != 429 && st < 500:
			return "", apiError(st, b)
		case err == nil:
			last = apiError(st, b)
		default:
			last = err
		}
		if session == "" {
			c.sleep(backoff(attempt))
		}
	}
	if session == "" {
		return "", last
	}
	if size == 0 {
		st, _, b, err := c.once(ctx, "PUT", session, nil, nil, false)
		if err != nil || st > 299 {
			return "", errors.Join(err, apiError(st, b))
		}
		return fileID(b)
	}
	buf := make([]byte, min(int64(chunk), size))
	var off int64
	failures := 0
	for off < size {
		n := min(int64(len(buf)), size-off)
		if _, err := f.ReadAt(buf[:n], off); err != nil {
			return "", err
		}
		h := http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", off, off+n-1, size)}}
		st, hdr, b, err := c.once(ctx, "PUT", session, h, buf[:n], false)
		if err == nil && (st == 200 || st == 201) {
			return fileID(b)
		}
		if err == nil && st == 308 {
			off, failures = nextOffset(hdr, off+n), 0
			continue
		}
		if err == nil && st != 429 && st < 500 {
			return "", apiError(st, b) // 404 or 410 is a session that is gone: the caller tries the whole file again later
		}
		if failures++; failures > 6 || ctx.Err() != nil {
			return "", errors.Join(err, apiError(st, b))
		}
		c.sleep(backoff(failures))
		// Ask how much Google has.
		st, hdr, b, err = c.once(ctx, "PUT", session, http.Header{"Content-Range": {fmt.Sprintf("bytes */%d", size)}}, nil, false)
		switch {
		case err == nil && (st == 200 || st == 201):
			return fileID(b)
		case err == nil && st == 308:
			off = nextOffset(hdr, 0)
		}
	}
	return "", errors.New("the upload ended without Google confirming the file")
}

// nextOffset reads "Range: bytes=0-N" of a 308 answer: the next byte to send is N+1. With no Range nothing has arrived, so fallback.
func nextOffset(h http.Header, fallback int64) int64 {
	r := h.Get("Range")
	if i := strings.LastIndex(r, "-"); i >= 0 {
		if n, err := strconv.ParseInt(r[i+1:], 10, 64); err == nil {
			return n + 1
		}
	}
	return fallback
}

func fileID(b []byte) (string, error) {
	var r struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(b, &r) != nil || r.ID == "" {
		return "", errors.New("Google Drive did not return the id of the file")
	}
	return r.ID, nil
}

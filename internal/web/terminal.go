package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"mpn/internal/access"
	"mpn/internal/webauth"
)

// Dialer logs in as a website's account; panelssh.Client is the real one.
type Dialer interface {
	Dial(user string) (*ssh.Client, error)
}

// basePolicy is the content security policy of every page: nothing from outside, no inline script or style.
const basePolicy = "default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// stylePolicy is the policy of a page whose script makes style elements while it runs (the terminal and the editor):
// exactly the ones that carry this page's one-time nonce are allowed, and nothing inline is.
func stylePolicy(nonce string) string {
	return strings.Replace(basePolicy, "style-src 'self'", "style-src 'self' 'nonce-"+nonce+"'", 1)
}

// terminalPolicy also lets the terminal page open a WebSocket to its own address.
func terminalPolicy(nonce string) string { return stylePolicy(nonce) + "; connect-src 'self'" }

func (s *Server) terminalPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /sites/{name}/terminal", s.auth(s.terminalPage))
	mux.HandleFunc("GET /sites/{name}/terminal/ws", s.auth(s.terminalSocket))
}

func (s *Server) terminalPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	nonce := base64.RawURLEncoding.EncodeToString(raw)
	w.Header().Set("Content-Security-Policy", terminalPolicy(nonce))
	s.show(w, 200, ss, view{Page: "site", Site: row, Tab: "terminal", Nonce: nonce, SSHEnabled: access.Enabled(s.passwdFile(), row.User)})
}

func clampInt(text string, def, lo, hi int) int {
	n, err := strconv.Atoi(text)
	if err != nil {
		return def
	}
	return min(max(n, lo), hi)
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (s *Server) termLog(event string, row siteRow, err error) {
	if s.TermLog != nil {
		s.TermLog(event, row.Name, err)
	}
}

// terminalSocket turns a WebSocket into a login shell of the website's account. The browser must prove it is the panel's
// own page three ways: the session cookie, the one-time form token in the address, and the Origin that the WebSocket
// library compares with the Host. Nothing that is typed or shown is kept.
func (s *Server) terminalSocket(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if c, err := r.Cookie(s.cookieName()); err != nil || !s.Sessions.CheckCSRF(c.Value, r.URL.Query().Get("csrf")) {
		http.Error(w, "invalid form token", http.StatusForbidden)
		return
	}
	if s.Dialer == nil {
		http.Error(w, T("err.nohelper"), http.StatusServiceUnavailable)
		return
	}
	limit := int32(8)
	if s.MaxTerminals > 0 {
		limit = int32(s.MaxTerminals)
	}
	if s.terms.Add(1) > limit {
		s.terms.Add(-1)
		http.Error(w, T("term.toomany"), http.StatusTooManyRequests)
		return
	}
	defer s.terms.Add(-1)
	q := r.URL.Query()
	cols, rows := clampInt(q.Get("cols"), 80, 20, 500), clampInt(q.Get("rows"), 24, 5, 200)
	// The HTTP server's read and write deadlines are meant for ordinary requests; a terminal lives for hours.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	conn, err := websocket.Accept(w, r, nil) // refuses a request whose Origin is not this host
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)
	s.runTerminalText(T, r.Context(), conn, row, cols, rows)
}
func (s *Server) runTerminal(parent context.Context, conn *websocket.Conn,
	row siteRow, cols, rows int) {
	s.runTerminalText(T, parent, conn, row, cols, rows)
}

func (s *Server) runTerminalText(T translator, parent context.Context, conn *websocket.Conn, row siteRow, cols, rows int) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	// Every normal ending goes through end, which sends a close frame and waits for the browser's answer. Cancelling the
	// context under a pending read would instead drop the connection without a word, and the page could not tell a clean
	// end from a failure.
	var once sync.Once
	end := func(code websocket.StatusCode, reason string) {
		once.Do(func() {
			_ = conn.Close(code, reason)
			cancel()
		})
	}
	idle := orDefault(s.TermIdle, 30*time.Minute)
	fail := func(text string, err error) {
		s.termLog("failed", row, err)
		b, _ := json.Marshal(map[string]string{"error": text})
		_ = conn.Write(ctx, websocket.MessageText, b)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	cli, err := s.Dialer.Dial(row.User)
	if err != nil {
		fail(T("term.loginfail"), err)
		return
	}
	defer cli.Close()
	sess, err := cli.NewSession()
	if err != nil {
		fail(T("term.loginfail"), err)
		return
	}
	defer sess.Close()
	if err := sess.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}); err != nil {
		fail(T("term.loginfail"), err)
		return
	}
	in, err1 := sess.StdinPipe()
	out, err2 := sess.StdoutPipe()
	if err1 != nil || err2 != nil || sess.Shell() != nil {
		fail(T("term.loginfail"), nil)
		return
	}
	s.termLog("open", row, nil)
	defer s.termLog("close", row, nil)

	idleTimer := time.AfterFunc(idle, func() { end(websocket.StatusNormalClosure, "idle") }) // when nobody types for a while
	defer idleTimer.Stop()
	limitTimer := time.AfterFunc(orDefault(s.TermMax, 8*time.Hour), func() { end(websocket.StatusNormalClosure, "time limit") })
	defer limitTimer.Stop()
	go func() { // keeps the connection alive through nginx and finds a browser that has gone
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, done := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Ping(pctx)
				done()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	go func() { // what the shell prints goes to the browser; when the shell ends, the whole thing ends
		defer end(websocket.StatusNormalClosure, "exit")
		buf := make([]byte, 32<<10)
		for {
			n, err := out.Read(buf)
			if n > 0 && conn.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for { // what the browser sends goes to the shell: binary is typing, text is a control message
		typ, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		idleTimer.Reset(idle)
		if typ == websocket.MessageBinary {
			if _, err := in.Write(data); err != nil {
				break
			}
			continue
		}
		var m struct {
			Type       string `json:"type"`
			Cols, Rows int
		}
		if json.Unmarshal(data, &m) == nil && m.Type == "resize" {
			_ = sess.WindowChange(clampInt(strconv.Itoa(m.Rows), 24, 5, 200), clampInt(strconv.Itoa(m.Cols), 80, 20, 500))
		}
	}
	end(websocket.StatusNormalClosure, "")
}

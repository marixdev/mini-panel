package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// What other machines send to the hub (their reports and a pairing) comes to the panel's own port, at /_notify/pair and /_notify/beat,
// and the panel passes it to the mpn-notify service on this machine. These two addresses need no sign-in: an agent proves itself with
// its own key, which the service checks. The panel only passes the body through, with the address of the sender, and adds nothing.
func (s *Server) notifyHubRoutes(mux routeMux) {
	if s.NotifySocket == "" {
		return
	}
	for _, p := range []string{"pair", "beat"} {
		mux.HandleFunc("POST /_notify/"+p, s.notifyRelay(p))
	}
}

// notifyRelayLimit bounds what an agent may send; a report with fifty messages is well under it.
const notifyRelayLimit = 256 << 10

func (s *Server) notifyRelay(path string) http.HandlerFunc {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", s.NotifySocket)
	}}
	c := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "JSON only", http.StatusUnsupportedMediaType)
			return
		}
		// A pairing is a guess of a secret of 32 random bytes; it is still counted like a sign-in, so nobody can try codes for ever.
		if path == "pair" {
			key := "pair|" + s.clientAddr(r)
			if wait := s.Throttle.Wait(key); wait > 0 {
				http.Error(w, "too many attempts", http.StatusTooManyRequests)
				return
			}
			s.Throttle.Fail(key)
		}
		req, _ := http.NewRequestWithContext(r.Context(), "POST", "http://hub/"+path, io.LimitReader(r.Body, notifyRelayLimit))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", s.clientAddr(r))
		resp, err := c.Do(req)
		if err != nil {
			http.Error(w, `{"error":"the alerts service of the hub is not running"}`, http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(resp.Body, notifyRelayLimit))
	}
}

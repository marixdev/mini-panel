package webauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"
)

// Session is what the server knows about a logged-in browser. The CSRF token is separate from the session cookie: a
// page includes it in every form, and a request that changes something must send it back.
type Session struct {
	User    string
	CSRF    string
	created time.Time
	last    time.Time
}

// Sessions keeps logins in memory. They are lost when the panel restarts, which only means logging in again.
// The map is keyed by a hash of the cookie value, so the values themselves are not kept anywhere.
type Sessions struct {
	Idle  time.Duration // a session unused for this long ends
	Max   time.Duration // no session lasts longer than this, however busy
	Limit int           // most sessions at once; the least recently used one makes room

	mu  sync.Mutex
	m   map[[32]byte]*Session
	now func() time.Time
}

func NewSessions(idle, max time.Duration, limit int) *Sessions {
	return &Sessions{Idle: idle, Max: max, Limit: limit, m: map[[32]byte]*Session{}, now: time.Now}
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("no random numbers: " + err.Error()) // the system is unusable; do not hand out guessable tokens
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func keyOf(token string) [32]byte { return sha256.Sum256([]byte(token)) }

func (s *Sessions) expired(x *Session, now time.Time) bool {
	return now.Sub(x.last) > s.Idle || now.Sub(x.created) > s.Max
}

// Create starts a session and returns the cookie value to give to the browser.
func (s *Sessions) Create(user string) (token string, sess Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, x := range s.m {
		if s.expired(x, now) {
			delete(s.m, k)
		}
	}
	for len(s.m) >= s.Limit && len(s.m) > 0 {
		var oldest [32]byte
		var at time.Time
		first := true
		for k, x := range s.m {
			if first || x.last.Before(at) {
				oldest, at, first = k, x.last, false
			}
		}
		delete(s.m, oldest)
	}
	token = randomToken()
	x := &Session{User: user, CSRF: randomToken(), created: now, last: now}
	s.m[keyOf(token)] = x
	return token, *x
}

// Get returns the session for a cookie value and counts the request as use. It forgets an expired session.
func (s *Sessions) Get(token string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := keyOf(token)
	x, ok := s.m[k]
	if !ok {
		return Session{}, false
	}
	now := s.now()
	if s.expired(x, now) {
		delete(s.m, k)
		return Session{}, false
	}
	x.last = now
	return *x, true
}

func (s *Sessions) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, keyOf(token))
}

// CheckCSRF reports whether got is the CSRF token of the session that token names.
func (s *Sessions) CheckCSRF(token, got string) bool {
	x, ok := s.Get(token)
	return ok && got != "" && subtle.ConstantTimeCompare([]byte(x.CSRF), []byte(got)) == 1
}

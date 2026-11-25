package webauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Session is what the server knows about a logged-in browser. The CSRF token is separate from the session cookie: a
// page includes it in every form, and a request that changes something must send it back.
type Session struct {
	User string
	CSRF string
	// Long is a session of a browser the person asked the panel to remember: its cookie outlives the browser, and it ends only after
	// LongIdle without use or LongMax in all.
	Long    bool
	created time.Time
	last    time.Time
	saved   time.Time // when last was last written to the file
}

// Sessions keeps logins. With File set they are also written there, so a restart of the panel (an update) does not sign anybody
// out. The map and the file are keyed by a hash of the cookie value, so the values themselves are not kept anywhere.
type Sessions struct {
	Idle     time.Duration // a session unused for this long ends
	Max      time.Duration // no session lasts longer than this, however busy
	LongIdle time.Duration // the same two for a remembered browser
	LongMax  time.Duration
	Limit    int    // most sessions at once; the least recently used one makes room
	File     string // where sessions are kept between restarts; "" keeps them in memory only

	mu  sync.Mutex
	m   map[[32]byte]*Session
	now func() time.Time
}

func NewSessions(idle, max time.Duration, limit int) *Sessions {
	return &Sessions{Idle: idle, Max: max, LongIdle: idle, LongMax: max, Limit: limit, m: map[[32]byte]*Session{}, now: time.Now}
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
	idle, max := s.Idle, s.Max
	if x.Long {
		idle, max = s.LongIdle, s.LongMax
	}
	return now.Sub(x.last) > idle || now.Sub(x.created) > max
}

// Lifetime is how long the cookie of a session should be kept by the browser: 0 for one that ends with the browser.
func (s *Sessions) Lifetime(x Session) time.Duration {
	if x.Long {
		return s.LongMax
	}
	return 0
}

// Create starts a session and returns the cookie value to give to the browser.
func (s *Sessions) Create(user string, long bool) (token string, sess Session) {
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
	x := &Session{User: user, CSRF: randomToken(), Long: long, created: now, last: now, saved: now}
	s.m[keyOf(token)] = x
	s.save()
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
		s.save()
		return Session{}, false
	}
	x.last = now
	// The time of use is written at most once a minute per session; a restart can lose that minute, no more.
	if now.Sub(x.saved) >= time.Minute {
		x.saved = now
		s.save()
	}
	return *x, true
}

func (s *Sessions) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, keyOf(token))
	s.save()
}

// DeleteExcept ends every session but the one that token names. A changed password calls it, so a browser that was signed in with the
// old password does not stay signed in.
func (s *Sessions) DeleteExcept(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := keyOf(token)
	for k := range s.m {
		if k != keep {
			delete(s.m, k)
		}
	}
	s.save()
}

// Count is the number of sessions that have not ended.
func (s *Sessions) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now, n := s.now(), 0
	for _, x := range s.m {
		if !s.expired(x, now) {
			n++
		}
	}
	return n
}

// CheckCSRF reports whether got is the CSRF token of the session that token names.
func (s *Sessions) CheckCSRF(token, got string) bool {
	x, ok := s.Get(token)
	return ok && got != "" && subtle.ConstantTimeCompare([]byte(x.CSRF), []byte(got)) == 1
}

type savedSession struct {
	Key     string    `json:"key"` // hex of the hash of the cookie value
	User    string    `json:"user"`
	CSRF    string    `json:"csrf"`
	Long    bool      `json:"long,omitempty"`
	Created time.Time `json:"created"`
	Last    time.Time `json:"last"`
}

// save writes the sessions to File. It is called with mu held. A file that cannot be written is logged: the sessions go on in
// memory, and only a restart would end them.
func (s *Sessions) save() {
	if s.File == "" {
		return
	}
	now := s.now()
	list := make([]savedSession, 0, len(s.m))
	for k, x := range s.m {
		if s.expired(x, now) {
			continue
		}
		list = append(list, savedSession{Key: hex.EncodeToString(k[:]), User: x.User, CSRF: x.CSRF, Long: x.Long, Created: x.created, Last: x.last})
	}
	b, err := json.Marshal(list)
	if err == nil {
		err = writePrivate(s.File, b)
	}
	if err != nil {
		log.Printf("sessions could not be saved: %v", err)
	}
}

func writePrivate(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sessions-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Load reads the sessions that File holds. A missing file is no session; a damaged one is set aside and is no session either.
func (s *Sessions) Load() {
	if s.File == "" {
		return
	}
	b, err := os.ReadFile(s.File)
	if err != nil {
		return
	}
	var list []savedSession
	if err := json.Unmarshal(b, &list); err != nil {
		log.Printf("sessions file is damaged, everybody signs in again: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, x := range list {
		raw, err := hex.DecodeString(x.Key)
		if err != nil || len(raw) != 32 || x.User == "" || x.CSRF == "" {
			continue
		}
		var k [32]byte
		copy(k[:], raw)
		v := &Session{User: x.User, CSRF: x.CSRF, Long: x.Long, created: x.Created, last: x.Last, saved: x.Last}
		if !s.expired(v, now) && len(s.m) < s.Limit {
			s.m[k] = v
		}
	}
}

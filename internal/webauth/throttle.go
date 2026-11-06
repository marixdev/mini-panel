package webauth

import (
	"sync"
	"time"
)

// Throttle slows down password guessing. After Free failed logins from one key (an address) that key is locked out for
// Base, and every further failure doubles the lock up to Max. A success clears the key.
//
// Memory is bounded: when Cap keys are tracked and none has expired, new keys share one bucket. That fails closed:
// an attacker using very many addresses can lock the unknown addresses out together, which costs only a delay for
// someone logging in from a new address, and never opens a way in.
type Throttle struct {
	Free int
	Base time.Duration
	Max  time.Duration
	Keep time.Duration // a key is forgotten this long after its last failure
	Cap  int

	mu  sync.Mutex
	m   map[string]*attempt
	now func() time.Time
}

type attempt struct {
	fails       int
	until, last time.Time
}

func NewThrottle() *Throttle {
	return &Throttle{Free: 5, Base: 30 * time.Second, Max: 15 * time.Minute, Keep: time.Hour, Cap: 10000,
		m: map[string]*attempt{}, now: time.Now}
}

// bucket returns the key to use for the caller, which is "*" when the table is full. Callers hold t.mu.
func (t *Throttle) bucket(key string, now time.Time) string {
	if _, ok := t.m[key]; ok || len(t.m) < t.Cap {
		return key
	}
	for k, a := range t.m {
		if now.Sub(a.last) > t.Keep && !now.Before(a.until) {
			delete(t.m, k)
		}
	}
	if len(t.m) < t.Cap {
		return key
	}
	return "*"
}

// Wait returns how long the key must still wait before it may try again, or 0.
func (t *Throttle) Wait(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if a, ok := t.m[t.bucket(key, now)]; ok {
		if d := a.until.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func (t *Throttle) Fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	k := t.bucket(key, now)
	a := t.m[k]
	if a == nil {
		a = &attempt{}
		t.m[k] = a
	}
	a.fails++
	a.last = now
	if a.fails > t.Free {
		shift := a.fails - t.Free - 1
		if shift > 20 {
			shift = 20
		}
		d := t.Base << shift
		if d <= 0 || d > t.Max {
			d = t.Max
		}
		a.until = now.Add(d)
	}
}

func (t *Throttle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, key)
}

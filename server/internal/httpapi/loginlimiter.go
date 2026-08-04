package httpapi

import (
	"sync"
	"time"
)

// loginLimiter slows down password guessing. It is deliberately in-process and
// per-client: a single Ptium replica is the unit that sees a burst, and the
// alternative — a shared counter in Postgres on the sign-in path — would turn
// every failed attempt into a write.
//
// Replicas therefore throttle independently. That is a documented trade-off: the
// limiter raises the cost of guessing rather than making it impossible, and
// bcrypt at cost 12 is what makes each attempt expensive.
type loginLimiter struct {
	mutex    sync.Mutex
	attempts map[string]*loginAttempts
	// Free attempts before backoff begins.
	threshold int
	// Backoff doubles from base up to ceiling.
	base    time.Duration
	ceiling time.Duration
	// Window is how long an idle client's history is kept.
	window time.Duration
	now    func() time.Time
}

type loginAttempts struct {
	failures int
	lastSeen time.Time
	blocked  time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		attempts:  map[string]*loginAttempts{},
		threshold: 5,
		base:      2 * time.Second,
		ceiling:   5 * time.Minute,
		window:    30 * time.Minute,
		now:       time.Now,
	}
}

// retryAfter reports how long a client must wait. Zero means it may try now.
func (limiter *loginLimiter) retryAfter(client string) time.Duration {
	if limiter == nil || client == "" {
		return 0
	}
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.sweep()
	entry, ok := limiter.attempts[client]
	if !ok {
		return 0
	}
	if remaining := entry.blocked.Sub(limiter.now()); remaining > 0 {
		return remaining
	}
	return 0
}

// fail records a rejected attempt and extends the client's backoff.
func (limiter *loginLimiter) fail(client string) {
	if limiter == nil || client == "" {
		return
	}
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.now()
	entry, ok := limiter.attempts[client]
	if !ok {
		entry = &loginAttempts{}
		limiter.attempts[client] = entry
	}
	entry.failures++
	entry.lastSeen = now
	if entry.failures <= limiter.threshold {
		return
	}
	delay := limiter.base << min(entry.failures-limiter.threshold-1, 10)
	if delay > limiter.ceiling || delay <= 0 {
		delay = limiter.ceiling
	}
	entry.blocked = now.Add(delay)
}

// succeed clears a client's history after a valid sign-in.
func (limiter *loginLimiter) succeed(client string) {
	if limiter == nil || client == "" {
		return
	}
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	delete(limiter.attempts, client)
}

// sweep drops clients that have been idle longer than the window. It runs on
// access rather than on a timer so the limiter needs no goroutine.
func (limiter *loginLimiter) sweep() {
	if len(limiter.attempts) == 0 {
		return
	}
	cutoff := limiter.now().Add(-limiter.window)
	for client, entry := range limiter.attempts {
		if entry.lastSeen.Before(cutoff) && entry.blocked.Before(limiter.now()) {
			delete(limiter.attempts, client)
		}
	}
}

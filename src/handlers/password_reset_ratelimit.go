package handlers

import (
	"sync"
	"time"
)

// maxResetBuckets caps the keyed limiter's map so a spray of distinct
// keys (many addresses / spoofed IPs) can't grow it without bound. When the cap
// is reached, fully-refilled idle buckets are swept before a new key is added.
const maxResetBuckets = 50_000

// keyedRateLimiter is an in-memory, per-instance token-bucket limiter keyed by
// an arbitrary string (an email address or a client IP). It backs the public
// password-reset request endpoint (CON-161): that endpoint sends mail on demand,
// so an unthrottled one is a way to flood an arbitrary inbox.
//
// Per-instance is sufficient for the current single-replica deploy and matches
// the existing Zernio connect-link limiter; a multi-replica deploy would move
// this to a shared store (Redis INCR + TTL). The clock is injectable so tests
// can advance time deterministically.
type keyedRateLimiter struct {
	capacity   float64
	refillRate float64 // tokens per second
	ttl        time.Duration

	mu      sync.Mutex
	buckets map[string]*rlBucket
	now     func() time.Time
}

type rlBucket struct {
	tokens   float64
	lastSeen time.Time
}

// newKeyedRateLimiter builds a limiter allowing `burst` requests per `window`
// per key, refilling continuously. A key idle for longer than 2×window has
// fully refilled, so it can be dropped without changing behaviour.
func newKeyedRateLimiter(burst float64, window time.Duration) *keyedRateLimiter {
	return &keyedRateLimiter{
		capacity:   burst,
		refillRate: burst / window.Seconds(),
		ttl:        2 * window,
		buckets:    make(map[string]*rlBucket),
		now:        time.Now,
	}
}

// allow consumes one token for key. It returns (true, 0) when the request is
// permitted, or (false, retryAfter) when the key is exhausted — retryAfter is
// the wait until the next token, suitable for a Retry-After header.
func (l *keyedRateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxResetBuckets {
			l.sweep(now)
		}
		b = &rlBucket{tokens: l.capacity, lastSeen: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastSeen).Seconds()
		b.tokens += elapsed * l.refillRate
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
	}
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.refillRate * float64(time.Second))
	return false, wait
}

// sweep drops buckets untouched for longer than ttl. Such a bucket has fully
// refilled, so removing it is equivalent to leaving a full one behind — it just
// bounds memory. Called under l.mu.
func (l *keyedRateLimiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.lastSeen) > l.ttl {
			delete(l.buckets, k)
		}
	}
}

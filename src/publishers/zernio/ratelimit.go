package zernio

import (
	"sync"
	"time"
)

// connectLinkBurst is the per-instance bucket capacity from the ticket
// (30 connect-link requests per minute).
const (
	connectLinkBurst      = 30
	connectLinkRefillTime = time.Minute
)

// RateLimiter is a single-process token bucket. It is intentionally
// in-memory and per-instance, matching the ticket's "max 30 connect-link
// requests per minute per Ogen instance" requirement.
//
// TODO: when Ogen runs as more than one replica, replace this with a
// distributed limiter (Redis INCR + TTL, or a Lua script for atomic
// take-with-expiry). The Allow() contract is intentionally simple
// enough to swap implementations without changes at the call site.
type RateLimiter struct {
	capacity   float64
	refillRate float64 // tokens per second

	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
}

// NewConnectLinkRateLimiter returns the limiter sized for the
// connect-link endpoint (30/min, full bucket on construction).
func NewConnectLinkRateLimiter() *RateLimiter {
	return NewRateLimiter(float64(connectLinkBurst), float64(connectLinkBurst)/connectLinkRefillTime.Seconds())
}

// NewRateLimiter constructs a token bucket with the given capacity and
// refill rate (tokens/second). The bucket starts full so the first
// `capacity` requests succeed without waiting.
func NewRateLimiter(capacity, refillPerSecond float64) *RateLimiter {
	return &RateLimiter{
		capacity:   capacity,
		refillRate: refillPerSecond,
		tokens:     capacity,
		lastRefill: time.Now(),
	}
}

// Allow attempts to consume one token. On success returns (true, 0).
// On exhaustion returns (false, retryAfter) where retryAfter is the
// duration until the next token is available — suitable for the
// HTTP Retry-After header (caller can round up to whole seconds).
func (r *RateLimiter) Allow() (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.tokens += elapsed * r.refillRate
	if r.tokens > r.capacity {
		r.tokens = r.capacity
	}
	r.lastRefill = now

	if r.tokens >= 1 {
		r.tokens--
		return true, 0
	}
	needed := 1 - r.tokens
	wait := time.Duration(needed / r.refillRate * float64(time.Second))
	return false, wait
}

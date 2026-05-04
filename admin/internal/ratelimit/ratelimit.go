// Package ratelimit provides a simple in-memory IP-based rate limiter.
// Used to gate /api/auth/verify so a stolen/guessed admin token can't be
// brute-forced at machine speed.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Limiter enforces a sliding window of N attempts per IP within a duration.
type Limiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

// New creates a limiter. Typical args for auth: max=10, window=1*time.Minute.
func New(max int, window time.Duration) *Limiter {
	return &Limiter{
		attempts: make(map[string][]time.Time),
		max:      max,
		window:   window,
	}
}

// Allow returns true if the key (e.g. IP) is under the limit, false otherwise.
// Calling Allow also records this attempt.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Purge old attempts
	times := l.attempts[key]
	i := 0
	for ; i < len(times); i++ {
		if times[i].After(cutoff) {
			break
		}
	}
	times = times[i:]

	if len(times) >= l.max {
		l.attempts[key] = times
		return false
	}

	times = append(times, now)
	l.attempts[key] = times

	// Opportunistic cleanup: if this map has too many entries, purge all
	// that are empty. Prevents unbounded growth from ephemeral IPs.
	if len(l.attempts) > 10000 {
		for k, v := range l.attempts {
			if len(v) == 0 || v[len(v)-1].Before(cutoff) {
				delete(l.attempts, k)
			}
		}
	}
	return true
}

// Reset removes all attempts for a key. Called after successful auth to
// avoid locking out a legitimate user who made a few bad attempts first.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// ClientIP returns the request's RemoteAddr host.
//
// IMPORTANT: we deliberately do NOT honor X-Forwarded-For / X-Real-IP.
// Those headers are attacker-controlled: a client can set
// `X-Forwarded-For: 1.1.1.<rand>` to present a different "IP" each request
// and trivially bypass per-IP rate limiting.
//
// Since all requests to the admin backend arrive through our internal Caddy
// gateway, RemoteAddr is always the gateway's Docker IP. The rate limit is
// therefore effectively GLOBAL (N attempts per minute total, across all
// actual clients). For a single-admin-user tool that's exactly what we want:
// any brute force attempt bumps a single shared counter.
//
// If we ever need per-real-client limiting, configure Caddy to OVERWRITE
// X-Forwarded-For with the verified client IP (not append), then trust it
// here with a specific trusted-proxy allowlist.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

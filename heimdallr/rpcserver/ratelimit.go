//go:build web

package rpcserver

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// exchangeCodeRatePerMinute is how many ExchangeCode requests a single IP
	// can make per minute after the burst is exhausted.
	exchangeCodeRatePerMinute = 1
	// exchangeCodeBurst is the number of ExchangeCode requests an IP can make
	// in quick succession before the steady-state rate kicks in.
	exchangeCodeBurst = 5
	// rateLimiterTTL is how long an IP entry is kept after its last request.
	rateLimiterTTL = 10 * time.Minute
	// maxRequestBodyBytes is the maximum allowed request body size across all
	// endpoints. This guards against memory exhaustion from oversized payloads.
	maxRequestBodyBytes int64 = 1 << 20 // 1 MiB
)

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	limit    rate.Limit
	burst    int
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		limit:    r,
		burst:    burst,
	}
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.limiters[ip]
	if !ok {
		entry = &rateLimiterEntry{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

// cleanup removes IP entries that have not been seen within ttl.
func (l *ipRateLimiter) cleanup(ttl time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	for ip, entry := range l.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(l.limiters, ip)
		}
	}
}

// clientIP extracts the client IP from the request.
// It trusts X-Real-IP (set by nginx/Caddy) when present, and falls back to
// RemoteAddr. Note: only deploy behind a trusted reverse proxy; otherwise
// X-Real-IP can be spoofed by clients.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// newBodyLimitMiddleware returns an HTTP middleware that caps request bodies at
// maxRequestBodyBytes. Requests that exceed the limit receive 413.
func newBodyLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// newRateLimitMiddleware returns an HTTP middleware that enforces per-IP rate
// limiting on the given URL paths. Requests to other paths are passed through
// unchanged. Requests that exceed the limit receive 429 Too Many Requests.
func newRateLimitMiddleware(rl *ipRateLimiter, paths ...string) func(http.Handler) http.Handler {
	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pathSet[r.URL.Path] {
				ip := clientIP(r)
				if !rl.getLimiter(ip).Allow() {
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

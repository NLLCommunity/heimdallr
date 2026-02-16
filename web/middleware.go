package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/NLLCommunity/heimdallr/model"
)

const (
	exchangeCodeRatePerMinute = 1
	exchangeCodeBurst         = 5
	rateLimiterTTL            = 10 * time.Minute
	maxRequestBodyBytes       int64 = 1 << 20 // 1 MiB
)

// authMiddleware checks the session cookie and injects the session into context.
// Unauthenticated requests to protected paths are redirected to /login.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public paths.
		path := r.URL.Path
		if path == "/login" || path == "/callback" || path == "/" ||
			strings.HasPrefix(path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := model.GetSession(cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := setSession(r.Context(), session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bodyLimitMiddleware caps request bodies at maxRequestBodyBytes.
func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// --- Rate limiter ---

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

// rateLimitMiddleware applies per-IP rate limiting to the given paths.
func rateLimitMiddleware(rl *ipRateLimiter, paths ...string) func(http.Handler) http.Handler {
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

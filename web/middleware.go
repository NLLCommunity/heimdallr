package web

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/NLLCommunity/heimdallr/model"
)

const (
	exchangeCodeRatePerMinute       = 1
	exchangeCodeBurst               = 5
	rateLimiterTTL                  = 10 * time.Minute
	maxRequestBodyBytes       int64 = 1 << 20 // 1 MiB
)

// redirectToLogin issues an HX-Redirect for HTMX requests (HTMX swallows 3xx
// silently and would swap the login page HTML into a settings panel) and a
// normal 303 otherwise.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

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
			redirectToLogin(w, r)
			return
		}

		session, err := model.GetSession(cookie.Value)
		if err != nil {
			redirectToLogin(w, r)
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

// parseTrustedProxies converts a list of CIDR or bare-IP strings into prefixes.
// Bare IPs are widened to a host prefix (/32 or /128).
func parseTrustedProxies(cidrs []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, raw := range cidrs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			prefixes = append(prefixes, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(s); err == nil {
			prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		return nil, fmt.Errorf("invalid trusted_proxies entry %q", raw)
	}
	return prefixes, nil
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// clientIP extracts the client IP. Forwarded-IP headers (X-Real-IP and
// X-Forwarded-For) are honored only when the immediate connection comes from a
// proxy in the trusted list; otherwise they're ignored to prevent spoofing of
// the per-IP rate limiter.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	remote = remote.Unmap()
	if !isTrusted(remote, trusted) {
		return remote.String()
	}
	if h := strings.TrimSpace(r.Header.Get("X-Real-IP")); h != "" {
		if a, err := netip.ParseAddr(h); err == nil {
			return a.Unmap().String()
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Walk right-to-left, skipping known-trusted hops; the rightmost
		// untrusted address is the closest the proxy chain got to the client.
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			s := strings.TrimSpace(parts[i])
			a, err := netip.ParseAddr(s)
			if err != nil {
				continue
			}
			a = a.Unmap()
			if !isTrusted(a, trusted) {
				return a.String()
			}
		}
	}
	return remote.String()
}

// rateLimitMiddleware applies per-IP rate limiting to the given paths. The
// trusted list controls which proxies' forwarded-IP headers are honored when
// determining the client IP.
func rateLimitMiddleware(rl *ipRateLimiter, trusted []netip.Prefix, paths ...string) func(http.Handler) http.Handler {
	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pathSet[r.URL.Path] {
				ip := clientIP(r, trusted)
				if !rl.getLimiter(ip).Allow() {
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

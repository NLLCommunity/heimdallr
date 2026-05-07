package web

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/time/rate"

	"github.com/NLLCommunity/heimdallr/model"
)

const (
	exchangeCodeRatePerMinute = 1
	exchangeCodeBurst         = 5
	// Sandbox sends are admin-only but still rate-limited per session user
	// so a single admin can't drain the bot's Discord quota by spamming the
	// sandbox. ~10/min steady, burst 5 is generous for testing message
	// previews while clamping abuse.
	sandboxRatePerMinute       = 10
	sandboxBurst               = 5
	rateLimiterTTL             = 10 * time.Minute
	maxRequestBodyBytes  int64 = 1 << 20 // 1 MiB
)

// redirectToLogin steers an unauthenticated request to the login page in a
// way each kind of caller can actually handle:
//   - HTMX requests get HX-Redirect (HTMX swallows 3xx silently and would
//     otherwise swap the login page HTML into a settings panel).
//   - AJAX/fetch callers (X-Requested-With: XMLHttpRequest, or Accept asks
//     for JSON) get 401 with a small JSON body. Without this, fetch() follows
//     the 303 to /login and the JS sees a 200 HTML response, which it
//     mistakes for success.
//   - Browser navigations get a normal 303 to /login.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusOK)
		return
	}
	if isAJAXRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"not signed in","login_url":"/login"}`))
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// isAJAXRequest is true when the request signals that it expects a structured
// (JSON) response rather than HTML — either via the historical
// X-Requested-With: XMLHttpRequest marker or via Accept: application/json.
// Used by redirectToLogin so fetch() callers don't silently follow a 303 to
// the login page HTML.
func isAJAXRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// authMiddleware checks the session cookie and injects the session into context.
// Unauthenticated requests to protected paths are redirected to /login.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public paths. `/` is intentionally NOT public — it's
		// the post-login landing handler that decides /guilds vs /login based
		// on the session, which means it needs the session injected by this
		// middleware first.
		path := r.URL.Path
		if path == "/login" || path == "/callback" ||
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

type keyedRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	limit    rate.Limit
	burst    int
}

func newKeyedRateLimiter(r rate.Limit, burst int) *keyedRateLimiter {
	return &keyedRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		limit:    r,
		burst:    burst,
	}
}

func (l *keyedRateLimiter) getLimiter(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.limiters[key]
	if !ok {
		entry = &rateLimiterEntry{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.limiters[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

func (l *keyedRateLimiter) cleanup(ttl time.Duration) {
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
// Bare IPs are widened to a host prefix (/32 or /128). Each input element may
// itself contain multiple values separated by whitespace or commas, so the
// same setting works whether it arrives as a TOML list, a single env var like
// HEIMDALLR_WEB_TRUSTED_PROXIES="0.0.0.0/0 ::/0", or a comma-separated string.
func parseTrustedProxies(cidrs []string) ([]netip.Prefix, error) {
	splitDelim := func(r rune) bool { return r == ',' || unicode.IsSpace(r) }

	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, raw := range cidrs {
		for _, s := range strings.FieldsFunc(raw, splitDelim) {
			if p, err := netip.ParsePrefix(s); err == nil {
				prefixes = append(prefixes, p.Masked())
				continue
			}
			if a, err := netip.ParseAddr(s); err == nil {
				prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()))
				continue
			}
			return nil, fmt.Errorf("invalid trusted_proxies entry %q", s)
		}
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
		// If the entire chain is trusted (e.g. trusted_proxies covers
		// 0.0.0.0/0, ::/0 because we know we sit behind a single appending
		// proxy like Heroku's router), fall back to the rightmost parseable
		// entry — that's the IP the trusted edge appended, so any leading
		// client-spoofed values are correctly ignored.
		parts := strings.Split(xff, ",")
		var rightmost string
		for _, v := range slices.Backward(parts) {
			s := strings.TrimSpace(v)
			a, err := netip.ParseAddr(s)
			if err != nil {
				continue
			}
			a = a.Unmap()
			if rightmost == "" {
				rightmost = a.String()
			}
			if !isTrusted(a, trusted) {
				return a.String()
			}
		}
		if rightmost != "" {
			return rightmost
		}
	}
	return remote.String()
}

// rateLimitRule selects which (method, path) combinations the limiter applies
// to. Gating by method matters when GET and POST share a URL but only one is
// state-changing — e.g. POST /callback exchanges a login code, while GET
// /callback is a confirmation page that link previewers and refreshes hit and
// shouldn't drain the limiter budget.
type rateLimitRule struct {
	Method string
	Path   string
}

// rateLimitMiddleware applies per-IP rate limiting to the given (method, path)
// rules. The trusted list controls which proxies' forwarded-IP headers are
// honored when determining the client IP.
func rateLimitMiddleware(rl *keyedRateLimiter, trusted []netip.Prefix, rules ...rateLimitRule) func(http.Handler) http.Handler {
	ruleSet := make(map[rateLimitRule]bool, len(rules))
	for _, rule := range rules {
		ruleSet[rule] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ruleSet[rateLimitRule{Method: r.Method, Path: r.URL.Path}] {
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

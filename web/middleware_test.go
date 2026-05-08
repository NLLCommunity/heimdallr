package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRedirectToLogin_BrowserNavigationGets303(t *testing.T) {
	req := httptest.NewRequest("GET", "/guild/123", nil)
	rec := httptest.NewRecorder()
	redirectToLogin(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestRedirectToLogin_HTMXRequestGetsHXRedirect(t *testing.T) {
	req := httptest.NewRequest("POST", "/guild/123/settings/mod-channel", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	redirectToLogin(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("HX-Redirect"))
	assert.Empty(t, rec.Header().Get("Location"),
		"HX-Redirect path must not also issue a 3xx Location header (HTMX swallows 3xx silently)")
}

func TestRedirectToLogin_AJAXRequestGets401JSON(t *testing.T) {
	cases := []struct {
		name   string
		header http.Header
	}{
		{
			name:   "X-Requested-With: XMLHttpRequest",
			header: http.Header{"X-Requested-With": {"XMLHttpRequest"}},
		},
		{
			name:   "Accept: application/json",
			header: http.Header{"Accept": {"application/json"}},
		},
		{
			name:   "Accept includes application/json among others",
			header: http.Header{"Accept": {"text/html, application/json;q=0.9"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/guild/123/posts/1", nil)
			req.Header = tc.header
			rec := httptest.NewRecorder()
			redirectToLogin(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			assert.Contains(t, rec.Body.String(), `"login_url":"/login"`,
				"AJAX 401 body must point at the login URL so the JS can navigate")
			assert.Empty(t, rec.Header().Get("Location"),
				"AJAX path must not issue a 3xx Location — fetch() would silently follow it and the JS would mistake the login HTML for success")
		})
	}
}

func TestRedirectToLogin_HTMXBeatsAJAX(t *testing.T) {
	// HTMX requests can also be sent with X-Requested-With (some setups), but
	// HX-Request is the more specific signal — HTMX needs HX-Redirect, not 401.
	req := httptest.NewRequest("POST", "/guild/123/settings/mod-channel", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rec := httptest.NewRecorder()
	redirectToLogin(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("HX-Redirect"))
}

func TestAuthMiddleware_SkipsPublicPaths(t *testing.T) {
	called := false
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// `/` is intentionally not public — handleRoot relies on the session
	// being injected by this middleware to decide /guilds vs /login.
	for _, path := range []string{"/login", "/callback", "/static/css/custom.css"} {
		called = false
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.True(t, called, "handler should be called for %s", path)
	}
}

// Regression for the bug where `/` was in the public skip list, so
// handleRoot always saw a nil session and redirected logged-in users back to
// /login. With `/` enforced by middleware, unauthenticated requests are
// bounced from middleware (here) and authenticated ones land in handleRoot
// with a real session.
func TestAuthMiddleware_RootRequiresAuth(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without a session")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestAuthMiddleware_RedirectsWithoutCookie(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/guilds", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

// HTMX swallows 3xx redirects silently and swaps the response body. For
// auth bounces we want the page to navigate, so emit HX-Redirect instead.
func TestAuthMiddleware_HTMXRequest_UsesHXRedirect(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/guilds", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "HTMX-aware redirect uses 200 + HX-Redirect, not 303")
	assert.Equal(t, "/login", rec.Header().Get("HX-Redirect"))
}

func TestBodyLimitMiddleware_LargeBody(t *testing.T) {
	handler := bodyLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, maxRequestBodyBytes+1)
		_, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	body := make([]byte, maxRequestBodyBytes+1)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRateLimiter_BlocksAfterBurst(t *testing.T) {
	rl := newKeyedRateLimiter(rate.Every(time.Minute), 2)

	handler := rateLimitMiddleware(rl, nil, rateLimitRule{Method: "GET", Path: "/callback"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests should succeed (burst = 2).
	for range 2 {
		req := httptest.NewRequest("GET", "/callback", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Third request should be rate limited.
	req := httptest.NewRequest("GET", "/callback", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimiter_IgnoresOtherPaths(t *testing.T) {
	rl := newKeyedRateLimiter(rate.Every(time.Minute), 1)

	handler := rateLimitMiddleware(rl, nil, rateLimitRule{Method: "GET", Path: "/callback"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up the limiter on /callback.
	req := httptest.NewRequest("GET", "/callback", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// /guilds should not be rate limited.
	req = httptest.NewRequest("GET", "/guilds", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// GET /callback (link previewers, refreshes) must not drain the limiter
// budget intended for POST /callback (the actual code exchange).
func TestRateLimiter_OnlyAppliesToConfiguredMethod(t *testing.T) {
	rl := newKeyedRateLimiter(rate.Every(time.Minute), 1)

	handler := rateLimitMiddleware(rl, nil, rateLimitRule{Method: "POST", Path: "/callback"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Many GETs from the same IP — none should consume the bucket.
	for range 5 {
		req := httptest.NewRequest("GET", "/callback", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "GET should not be rate-limited")
	}

	// First POST is the burst.
	req := httptest.NewRequest("POST", "/callback", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second POST is rate-limited (proves the GETs didn't consume budget).
	req = httptest.NewRequest("POST", "/callback", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestParseTrustedProxies(t *testing.T) {
	prefixes, err := parseTrustedProxies([]string{"127.0.0.1/32", " 10.0.0.0/8 ", "::1", ""})
	require.NoError(t, err)
	require.Len(t, prefixes, 3)

	_, err = parseTrustedProxies([]string{"not-a-cidr"})
	assert.Error(t, err)
}

// Env-var paths often arrive as a single string holding multiple CIDRs
// separated by whitespace or commas (Viper's slice-from-env conversion is
// finicky and operators tend to use commas regardless). Each input element
// must be split on those delimiters so the same setting works whether it
// comes from TOML or HEIMDALLR_WEB_TRUSTED_PROXIES="0.0.0.0/0 ::/0".
func TestParseTrustedProxies_SplitsWhitespaceAndCommas(t *testing.T) {
	prefixes, err := parseTrustedProxies([]string{"0.0.0.0/0 ::/0"})
	require.NoError(t, err)
	require.Len(t, prefixes, 2)

	prefixes, err = parseTrustedProxies([]string{"127.0.0.1/32,10.0.0.0/8, 192.168.0.0/16"})
	require.NoError(t, err)
	require.Len(t, prefixes, 3)

	// Mixed: a list whose entries are themselves multi-valued.
	prefixes, err = parseTrustedProxies([]string{"127.0.0.1/32", "10.0.0.0/8 ::1"})
	require.NoError(t, err)
	require.Len(t, prefixes, 3)

	// One bad token in the middle of a multi-valued entry still fails.
	_, err = parseTrustedProxies([]string{"127.0.0.1/32 not-a-cidr"})
	assert.Error(t, err)
}

// Untrusted clients must not be able to spoof X-Real-IP / X-Forwarded-For;
// otherwise each forged header value gets its own rate-limit bucket and the
// limiter is bypassed.
func TestRateLimiter_IgnoresSpoofedForwardedHeaders(t *testing.T) {
	rl := newKeyedRateLimiter(rate.Every(time.Minute), 1)

	// trustedProxies = nil → forwarded headers are never honored.
	handler := rateLimitMiddleware(rl, nil, rateLimitRule{Method: "GET", Path: "/callback"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from real client succeeds (burst = 1).
	req := httptest.NewRequest("GET", "/callback", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Real-IP", "9.9.9.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Same RemoteAddr, attacker rotates X-Real-IP to dodge the bucket.
	// Limiter must still see the same RemoteAddr-keyed bucket.
	req = httptest.NewRequest("GET", "/callback", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Real-IP", "9.9.9.2")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// When a trusted proxy connects, the rate limiter must key off the forwarded
// client IP — otherwise every request through the proxy shares one bucket.
func TestRateLimiter_HonorsTrustedProxyXRealIP(t *testing.T) {
	rl := newKeyedRateLimiter(rate.Every(time.Minute), 1)
	trusted, err := parseTrustedProxies([]string{"127.0.0.1/32"})
	require.NoError(t, err)

	handler := rateLimitMiddleware(rl, trusted, rateLimitRule{Method: "GET", Path: "/callback"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, ip := range []string{"9.9.9.1", "9.9.9.2"} {
		req := httptest.NewRequest("GET", "/callback", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Real-IP", ip)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "IP %s should have its own bucket", ip)
	}

	// Reusing the first forwarded IP exhausts that bucket.
	req := httptest.NewRequest("GET", "/callback", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "9.9.9.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestClientIP_XForwardedForRightmostUntrusted(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"127.0.0.1/32", "10.0.0.0/8"})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	// Chain: client → external proxy 198.51.100.5 → internal proxy 10.0.0.2 → us.
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.5, 10.0.0.2")

	got := clientIP(req, trusted)
	assert.Equal(t, "198.51.100.5", got, "rightmost untrusted hop is the closest known client")
}

// When the operator trusts the entire address space — the recommended setup
// for Heroku, where the router appends to X-Forwarded-For but its IPs aren't
// stable — clientIP must return the rightmost XFF entry (the IP the trusted
// edge appended) rather than collapsing to the immediate proxy's RemoteAddr.
// Any leading client-supplied values must still be ignored.
func TestClientIP_TrustAllReturnsRightmostXFF(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"0.0.0.0/0", "::/0"})
	require.NoError(t, err)

	t.Run("single hop appended by trusted edge", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:1234"
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		assert.Equal(t, "203.0.113.9", clientIP(req, trusted))
	})

	t.Run("client-spoofed leading entries ignored", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:1234"
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8, 203.0.113.9")
		assert.Equal(t, "203.0.113.9", clientIP(req, trusted))
	})
}

func TestClientIP_MalformedXRealIPIgnored(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"127.0.0.1/32"})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "not-an-ip")

	assert.Equal(t, "127.0.0.1", clientIP(req, trusted))
}

// Sanity check that prefix-membership works for both v4 and v6.
func TestIsTrusted(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"10.0.0.0/8", "::1"})
	require.NoError(t, err)

	cases := map[string]bool{
		"10.1.2.3":  true,
		"11.0.0.1":  false,
		"::1":       true,
		"127.0.0.1": false,
	}
	for s, want := range cases {
		got := isTrusted(netip.MustParseAddr(s), trusted)
		assert.Equal(t, want, got, "isTrusted(%s)", s)
	}
}

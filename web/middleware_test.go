package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestAuthMiddleware_SkipsPublicPaths(t *testing.T) {
	called := false
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/login", "/callback", "/", "/static/css/custom.css"} {
		called = false
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.True(t, called, "handler should be called for %s", path)
	}
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
	rl := newIPRateLimiter(rate.Every(time.Minute), 2)

	handler := rateLimitMiddleware(rl, "/callback")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests should succeed (burst = 2).
	for i := 0; i < 2; i++ {
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
	rl := newIPRateLimiter(rate.Every(time.Minute), 1)

	handler := rateLimitMiddleware(rl, "/callback")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

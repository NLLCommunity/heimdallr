package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// canonicalOrigin must strip default ports for the scheme so a base_url like
// "https://example.com:443" matches what browsers actually send in Origin
// (which omits the default port). Non-default ports must be preserved.
func TestCanonicalOrigin(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://example.com", "https://example.com"},
		{"https://example.com:443", "https://example.com"},
		{"http://example.com:80", "http://example.com"},
		{"http://example.com:443", "http://example.com:443"},     // non-default port for http
		{"https://example.com:8443", "https://example.com:8443"}, // non-default port preserved
		{"http://localhost:8484", "http://localhost:8484"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			assert.Equal(t, tc.want, canonicalOrigin(u))
		})
	}
}

// MaxAge=0 in the cookie struct omits the attribute on the wire and leaves
// the cookie as a session cookie that some browsers persist across
// tab-restore. handleLogout must emit `Max-Age=0` so the browser deletes it.
func TestHandleLogout_ClearsCookieWithMaxAgeZero(t *testing.T) {
	req := httptest.NewRequest("GET", "/logout", nil)
	rec := httptest.NewRecorder()

	handleLogout(false)(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)

	var setCookie string
	for _, h := range rec.Header().Values("Set-Cookie") {
		if strings.HasPrefix(h, sessionCookieName+"=") {
			setCookie = h
			break
		}
	}
	if assert.NotEmpty(t, setCookie, "logout must emit a Set-Cookie for the session cookie") {
		assert.Contains(t, setCookie, sessionCookieName+"=;",
			"cookie value must be empty")
		assert.Contains(t, setCookie, "Max-Age=0",
			"Max-Age=0 is required for the browser to delete the cookie")
	}
}

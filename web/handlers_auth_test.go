package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testOrigin = "https://dashboard.example.com"

func TestIsSameOriginPost(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		referer string
		want    bool
	}{
		{name: "origin matches", origin: testOrigin, want: true},
		{name: "origin mismatches", origin: "https://evil.example.com", want: false},
		{name: "origin matches scheme-sensitive", origin: "http://dashboard.example.com", want: false},
		{name: "origin missing, referer matches", referer: testOrigin + "/callback?code=abc", want: true},
		{name: "origin missing, referer mismatches", referer: "https://evil.example.com/x", want: false},
		{name: "origin missing, referer is path-only (no host)", referer: "/callback", want: false},
		{name: "origin missing, referer is malformed", referer: "://broken", want: false},
		{name: "both missing", want: false},
		{name: "origin takes precedence over referer", origin: "https://evil.example.com", referer: testOrigin, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/callback", strings.NewReader("code=x"))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			assert.Equal(t, tc.want, isSameOriginPost(req, testOrigin))
		})
	}
}

// TestHandleCallbackPOST_RejectsCrossOrigin verifies the handler short-circuits
// before reaching ExchangeLoginCode when the Origin doesn't match. The DB is
// never touched (no model.DB set up in this test), so a cookie-bearing
// response would only happen if the early return failed.
func TestHandleCallbackPOST_RejectsCrossOrigin(t *testing.T) {
	handler := handleCallbackPOST(testOrigin)

	cases := []struct {
		name   string
		header http.Header
	}{
		{
			name:   "mismatched Origin",
			header: http.Header{"Origin": {"https://evil.example.com"}},
		},
		{
			name:   "no Origin and no Referer",
			header: http.Header{},
		},
		{
			name:   "Origin missing, Referer mismatched",
			header: http.Header{"Referer": {"https://evil.example.com/x"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{"code": {"any-code"}}
			req := httptest.NewRequest("POST", "/callback", strings.NewReader(form.Encode()))
			req.Header = tc.header
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, "/login", rec.Header().Get("Location"))
			assert.Empty(t, rec.Header().Values("Set-Cookie"),
				"rejected request must not set a session cookie")
		})
	}
}

package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/assert"
)

// A token exchange must grant every scope the dashboard needs - a
// mid-flow scope=identify edit would otherwise mint a session whose
// /guilds loads all fail downstream.
func TestHasRequiredScopes(t *testing.T) {
	assert.True(t, hasRequiredScopes([]discord.OAuth2Scope{
		discord.OAuth2ScopeIdentify, discord.OAuth2ScopeGuilds,
	}))
	// Extra scopes are fine; missing ones are not.
	assert.True(t, hasRequiredScopes([]discord.OAuth2Scope{
		discord.OAuth2ScopeGuilds, discord.OAuth2ScopeIdentify, discord.OAuth2ScopeEmail,
	}))
	assert.False(t, hasRequiredScopes([]discord.OAuth2Scope{discord.OAuth2ScopeIdentify}))
	assert.False(t, hasRequiredScopes([]discord.OAuth2Scope{discord.OAuth2ScopeGuilds}))
	assert.False(t, hasRequiredScopes(nil))
}

// The state cookie carries every outstanding flow's state so parallel
// login tabs don't invalidate each other; what one request writes the
// callback must read back intact.
func TestOAuthStateCookie_RoundTrip(t *testing.T) {
	req := httptest.NewRequest("GET", "/oauth/callback", nil)
	req.AddCookie(makeOAuthStateCookie([]string{"s1", "s2"}, false))
	assert.Equal(t, []string{"s1", "s2"}, readOAuthStateCookie(req))

	assert.Nil(t, readOAuthStateCookie(httptest.NewRequest("GET", "/oauth/callback", nil)),
		"absent cookie must read as no outstanding states")
}

// An empty state list must produce the expiring form of the cookie so
// the browser deletes it, and both forms must agree on Name and Path -
// otherwise the clear never removes what the set created.
func TestOAuthStateCookie_EmptyListExpires(t *testing.T) {
	set := makeOAuthStateCookie([]string{"s1"}, true)
	clear := makeOAuthStateCookie(nil, true)
	assert.Equal(t, -1, clear.MaxAge)
	assert.Positive(t, set.MaxAge)
	assert.Equal(t, set.Name, clear.Name)
	assert.Equal(t, set.Path, clear.Path)
}

// safeReturnTo gates what's allowed back through the OAuth dance. The
// goal is to deep-link dashboard URLs the user just clicked, not to
// honor arbitrary redirect targets — which would be an open-redirect.
// Anything not starting with "/guild/" or carrying a scheme/host must
// be refused.
func TestSafeReturnTo(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Empty round-trips to empty — the caller defaults to /guilds.
		{"", ""},
		// Allowed: relative dashboard paths.
		{"/guild/12345", "/guild/12345"},
		{"/guild/12345/posts", "/guild/12345/posts"},
		{"/guild/12345/posts?foo=bar", "/guild/12345/posts?foo=bar"},
		// Refused: absolute URLs / scheme present.
		{"https://evil.example.com/guild/12345", ""},
		{"//evil.example.com/guild/12345", ""},
		// Refused: paths outside the /guild/ prefix.
		{"/static/foo.js", ""},
		{"/login", ""},
		{"/", ""},
		// Refused: path traversal. Browsers resolve "/guild/../login"
		// client-side to "/login", which is same-origin but not what
		// the /guild/ allowlist was meant to cover. path.Clean catches
		// these before the prefix check.
		{"/guild/../login", ""},
		{"/guild/../../etc/passwd", ""},
		{"/guild/12345/../../static/foo.js", ""},
		// Refused: raw control characters never survive url.Parse.
		{"/guild/12345\r\nX-Evil: x", ""},
		// Neutralized: percent-encoded control characters decode into
		// the parsed path, but the output is rebuilt through url.URL,
		// which re-encodes them - no raw byte can split a header. The
		// old hand-rolled denylist only covered \x00, \r, and \n and
		// emitted other C0 bytes (like the tab below) raw.
		{"/guild/12345%0d%0aX-Evil:%20x", "/guild/12345%0D%0AX-Evil:%20x"},
		{"/guild/12345%09tab", "/guild/12345%09tab"},
		{"/guild/12345%7f", "/guild/12345%7F"},
		// Allowed but normalized: redundant "." and "//" segments are
		// cleaned through, the result must still start with /guild/.
		{"/guild/./12345", "/guild/12345"},
		{"/guild//12345", "/guild/12345"},
		// Refused: oversized input. The value is persisted into
		// dashboard_oauth_states, so inputs past the cap are rejected
		// even when otherwise well-formed.
		{"/guild/12345/" + strings.Repeat("a", maxReturnToLength), ""},
		// Boundary: exactly at the cap is still accepted.
		{"/guild/" + strings.Repeat("a", maxReturnToLength-7), "/guild/" + strings.Repeat("a", maxReturnToLength-7)},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := safeReturnTo(tc.in)
			if got != tc.want {
				t.Fatalf("safeReturnTo(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

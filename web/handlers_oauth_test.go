package web

import "testing"

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
		// Refused: control characters that could split headers.
		{"/guild/12345\r\nX-Evil: x", ""},
		// Allowed but normalized: redundant "." and "//" segments are
		// cleaned through, the result must still start with /guild/.
		{"/guild/./12345", "/guild/12345"},
		{"/guild//12345", "/guild/12345"},
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

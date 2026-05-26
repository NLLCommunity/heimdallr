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
		// Refused: control characters that could split headers.
		{"/guild/12345\r\nX-Evil: x", ""},
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

package listeners

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateContent_AsciiUnderLimit(t *testing.T) {
	got := truncateContent("hello", 10)
	assert.Equal(t, "hello", got)
}

func TestTruncateContent_AsciiOverLimit(t *testing.T) {
	got := truncateContent("abcdefghij", 5)
	// Marker counts toward the 5-rune budget: 4 content runes + "…" = 5 runes.
	assert.Equal(t, "abcd"+truncationMarker, got)
	assert.Equal(t, 5, utf8.RuneCountInString(got))
}

// A long string of 4-byte runes (𝓐 = U+1D4D0) — byte-slicing at an arbitrary
// index would corrupt the codepoint. truncateContent must cut on a rune
// boundary.
func TestTruncateContent_MultiByteRune_CutsOnBoundary(t *testing.T) {
	const r = "𝓐" // 4 bytes
	in := ""
	for i := 0; i < 10; i++ {
		in += r
	}
	got := truncateContent(in, 5)
	assert.Equal(t, 5, utf8.RuneCountInString(got), "result must not exceed maxRunes")
	assert.True(t, utf8.ValidString(got), "result must be valid UTF-8")
}

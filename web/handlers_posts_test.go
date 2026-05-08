package web

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcquirePublishLock_SerializesPerPost(t *testing.T) {
	const postID uint = 999_999_001

	first, ok1 := acquirePublishLock(postID)
	assert.True(t, ok1, "first acquire must succeed")

	_, ok2 := acquirePublishLock(postID)
	assert.False(t, ok2, "second concurrent acquire must fail (TryLock contention)")

	first.Unlock()

	third, ok3 := acquirePublishLock(postID)
	assert.True(t, ok3, "after release, a fresh acquire must succeed")
	third.Unlock()
}

func TestAcquirePublishLock_DifferentPostsDoNotInterfere(t *testing.T) {
	const a uint = 999_999_010
	const b uint = 999_999_011

	muA, okA := acquirePublishLock(a)
	muB, okB := acquirePublishLock(b)
	assert.True(t, okA)
	assert.True(t, okB, "lock on a different post ID must be independent")

	muA.Unlock()
	muB.Unlock()
}

func TestValidatePostComponents(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string // substring; "" means no error expected
	}{
		{
			name: "empty array is valid",
			in:   "[]",
		},
		{
			name: "single text_display is valid",
			in:   `[{"type":10,"content":"hello"}]`,
		},
		{
			name: "container with children is valid",
			in:   `[{"type":17,"components":[{"type":10,"content":"hi"}]}]`,
		},
		{
			name:    "malformed JSON is rejected",
			in:      `[{"type":10`,
			wantErr: "invalid components JSON",
		},
		{
			name:    "non-JSON garbage is rejected",
			in:      "not json",
			wantErr: "invalid components JSON",
		},
		{
			name:    "object at top level is rejected",
			in:      `{"type":10,"content":"hi"}`,
			wantErr: "invalid components JSON",
		},
		{
			name:    "single text_display over 4000 chars is rejected at validate time",
			in:      `[{"type":10,"content":"` + strings.Repeat("a", 4001) + `"}]`,
			wantErr: "more than 4000",
		},
		{
			name: "text_display at exactly 4000 chars is accepted",
			in:   `[{"type":10,"content":"` + strings.Repeat("a", 4000) + `"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePostComponents(tc.in)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

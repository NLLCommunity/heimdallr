package web

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			wantErr: "exceeds 4000",
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

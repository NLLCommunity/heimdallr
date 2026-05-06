package web

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAndCompactV2JSON(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "compacts whitespace and preserves order",
			in:   `[ { "type": 10, "content": "hi" }, { "type": 1 } ]`,
			want: `[{"type":10,"content":"hi"},{"type":1}]`,
		},
		{
			name: "trims leading/trailing whitespace",
			in:   "   [ {\"type\":10} ]\n",
			want: `[{"type":10}]`,
		},
		{
			name:    "empty string is rejected",
			in:      "",
			wantErr: true,
		},
		{
			name:    "whitespace-only is rejected",
			in:      "   \n\t  ",
			wantErr: true,
		},
		{
			name:    "empty array is rejected",
			in:      "[]",
			wantErr: true,
		},
		{
			name:    "object at top level is rejected (handler expects array)",
			in:      `{"type":10}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON is rejected",
			in:      `[{"type":10`,
			wantErr: true,
		},
		{
			name:    "non-JSON garbage is rejected",
			in:      "not json at all",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateAndCompactV2JSON(tc.in)
			if tc.wantErr {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, errInvalidV2JSON),
					"all validation errors should wrap errInvalidV2JSON; got %v", err)
				assert.Empty(t, got)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// JSON numbers decoded into []any would coerce snowflakes (large 64-bit ints)
// to float64 and silently lose precision. Decoding into []json.RawMessage —
// what validateAndCompactV2JSON does — must round-trip them verbatim.
func TestValidateAndCompactV2JSON_PreservesLargeIntegers(t *testing.T) {
	in := `[{"id":1234567890123456789,"channel_id":987654321098765432}]`
	got, err := validateAndCompactV2JSON(in)
	assert.NoError(t, err)
	assert.Contains(t, got, "1234567890123456789",
		"snowflake-sized integer must round-trip without precision loss")
	assert.Contains(t, got, "987654321098765432")
}

func TestPreserveV2Json(t *testing.T) {
	t.Run("empty input returns empty", func(t *testing.T) {
		assert.Equal(t, "", preserveV2Json(""))
	})

	t.Run("oversized input returns empty", func(t *testing.T) {
		// One byte over the cap, syntactically valid otherwise — must still
		// be discarded so the size cap is the gate, not parsing.
		oversized := "[" + strings.Repeat(" ", maxV2JsonStored) + "]"
		assert.Equal(t, "", preserveV2Json(oversized))
	})

	t.Run("malformed input returns empty", func(t *testing.T) {
		assert.Equal(t, "", preserveV2Json("[{not json"))
	})

	t.Run("empty array returns empty", func(t *testing.T) {
		// validateAndCompactV2JSON rejects [], so preserveV2Json must too —
		// otherwise the toggle-off path would persist drafts the toggle-on
		// path refuses to save.
		assert.Equal(t, "", preserveV2Json("[]"))
	})

	t.Run("valid array is compacted", func(t *testing.T) {
		got := preserveV2Json(`[ { "type": 10 } ]`)
		assert.Equal(t, `[{"type":10}]`, got)
	})
}

func TestParseSnowflakeOrZero(t *testing.T) {
	t.Run("empty is treated as unset", func(t *testing.T) {
		id, err := parseSnowflakeOrZero("")
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), uint64(id))
	})

	t.Run("valid snowflake parses", func(t *testing.T) {
		id, err := parseSnowflakeOrZero("123456789012345678")
		assert.NoError(t, err)
		assert.Equal(t, uint64(123456789012345678), uint64(id))
	})

	t.Run("non-numeric input errors", func(t *testing.T) {
		_, err := parseSnowflakeOrZero("not-a-snowflake")
		assert.Error(t, err)
	})
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		name string
		in   string
		def  int
		want int
	}{
		{"valid integer", "42", 5, 42},
		{"trims whitespace", "  7\n", 5, 7},
		{"empty falls back to default", "", 5, 5},
		{"non-numeric falls back to default", "abc", 99, 99},
		{"float falls back to default", "3.14", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseInt(tc.in, tc.def))
		})
	}
}

func TestParseFloat(t *testing.T) {
	t.Run("valid float", func(t *testing.T) {
		v, err := parseFloat("3.14")
		assert.NoError(t, err)
		assert.InDelta(t, 3.14, v, 0.0001)
	})
	t.Run("integer parses as float", func(t *testing.T) {
		v, err := parseFloat("42")
		assert.NoError(t, err)
		assert.InDelta(t, 42.0, v, 0.0001)
	})
	t.Run("trims whitespace", func(t *testing.T) {
		v, err := parseFloat("  2.5\n")
		assert.NoError(t, err)
		assert.InDelta(t, 2.5, v, 0.0001)
	})
	t.Run("empty errors", func(t *testing.T) {
		_, err := parseFloat("")
		assert.Error(t, err)
	})
	t.Run("non-numeric errors", func(t *testing.T) {
		_, err := parseFloat("abc")
		assert.Error(t, err)
	})
}

// Sanity check: the bound constants in this file must stay in sync with the
// human-readable error messages in handleSaveAntiSpam / handleSaveInfractions.
// If someone bumps maxAntiSpamCount to 20 but forgets to update the "between
// 2 and 10" error text, this test fails loudly.
func TestSettingsBoundsMatchErrorMessages(t *testing.T) {
	assert.Equal(t, 2, minAntiSpamCount, "anti-spam error text says 'between 2 and 10'")
	assert.Equal(t, 10, maxAntiSpamCount, "anti-spam error text says 'between 2 and 10'")
	assert.Equal(t, 1, minAntiSpamCooldownSeconds, "cooldown error text says 'between 1 and 60'")
	assert.Equal(t, 60, maxAntiSpamCooldownSeconds, "cooldown error text says 'between 1 and 60'")
	assert.Equal(t, 0.0, minInfractionHalfLifeDays, "half-life error text says 'between 0 and 365'")
	assert.Equal(t, 365.0, maxInfractionHalfLifeDays, "half-life error text says 'between 0 and 365'")
	assert.Equal(t, 0.0, minNotifyWarnSeverityThreshold, "severity error text says 'between 0 and 100'")
	assert.Equal(t, 100.0, maxNotifyWarnSeverityThreshold, "severity error text says 'between 0 and 100'")
}

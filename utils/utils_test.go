package utils

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcHalfLife(t *testing.T) {
	tests := []struct {
		name         string
		timeSince    time.Duration
		halfLifeDays float64
		weight       float64
		expected     float64
	}{
		{
			name:         "exact half life",
			timeSince:    90 * 24 * time.Hour,
			halfLifeDays: 90,
			weight:       1.0,
			expected:     0.5,
		},
		{
			name:         "zero half life",
			timeSince:    100 * 24 * time.Hour,
			halfLifeDays: 0.0,
			weight:       1.0,
			expected:     1.0,
		},
		{
			name:         "zero time since",
			timeSince:    0,
			halfLifeDays: 90,
			weight:       1.0,
			expected:     1.0,
		},
		{
			name:         "double half life",
			timeSince:    180 * 24 * time.Hour,
			halfLifeDays: 90,
			weight:       1.0,
			expected:     0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalcHalfLife(tt.timeSince, tt.halfLifeDays, tt.weight)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestRef(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		s := "test"
		ptr := new(s)
		assert.Equal(t, &s, ptr)
		assert.Equal(t, s, *ptr)
	})

	t.Run("int", func(t *testing.T) {
		i := 42
		ptr := new(i)
		assert.Equal(t, &i, ptr)
		assert.Equal(t, i, *ptr)
	})
}

func TestWrapRef(t *testing.T) {
	t.Run("non-nil pointer", func(t *testing.T) {
		s := "test"
		value, ok := WrapRef(&s)
		assert.True(t, ok)
		assert.Equal(t, s, value)
	})

	t.Run("nil pointer", func(t *testing.T) {
		var ptr *string
		value, ok := WrapRef(ptr)
		assert.False(t, ok)
		assert.Equal(t, "", value) // zero value for string.
	})
}

func TestRefDefault(t *testing.T) {
	t.Run("non-nil pointer", func(t *testing.T) {
		result := RefDefault(new("test"), "default")
		assert.Equal(t, "test", result)
	})

	t.Run("nil pointer", func(t *testing.T) {
		var ptr *string
		result := RefDefault(ptr, "default")
		assert.Equal(t, "default", result)
	})
}

func TestIif(t *testing.T) {
	t.Run("true condition", func(t *testing.T) {
		result := Iif(true, "yes", "no")
		assert.Equal(t, "yes", result)
	})

	t.Run("false condition", func(t *testing.T) {
		result := Iif(false, "yes", "no")
		assert.Equal(t, "no", result)
	})
}

func TestAny(t *testing.T) {
	tests := []struct {
		name     string
		values   []bool
		expected bool
	}{
		{
			name:     "all false",
			values:   []bool{false, false, false},
			expected: false,
		},
		{
			name:     "some true",
			values:   []bool{false, true, false},
			expected: true,
		},
		{
			name:     "all true",
			values:   []bool{true, true, true},
			expected: true,
		},
		{
			name:     "empty",
			values:   []bool{},
			expected: false,
		},
		{
			name:     "single true",
			values:   []bool{true},
			expected: true,
		},
		{
			name:     "single false",
			values:   []bool{false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Any(tt.values...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAll(t *testing.T) {
	tests := []struct {
		name     string
		values   []bool
		expected bool
	}{
		{
			name:     "all false",
			values:   []bool{false, false, false},
			expected: false,
		},
		{
			name:     "some true",
			values:   []bool{false, true, false},
			expected: false,
		},
		{
			name:     "all true",
			values:   []bool{true, true, true},
			expected: true,
		},
		{
			name:     "empty",
			values:   []bool{},
			expected: true,
		},
		{
			name:     "single true",
			values:   []bool{true},
			expected: true,
		},
		{
			name:     "single false",
			values:   []bool{false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := All(tt.values...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseLongDuration(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:     "full format",
			input:    "1y2mo3w4d5h6m7s",
			expected: 365*24*time.Hour + 2*30*24*time.Hour + 3*7*24*time.Hour + 4*24*time.Hour + 5*time.Hour + 6*time.Minute + 7*time.Second,
		},
		{
			name:     "days only",
			input:    "30d",
			expected: 30 * 24 * time.Hour,
		},
		{
			name:     "hours and minutes",
			input:    "2h30m",
			expected: 2*time.Hour + 30*time.Minute,
		},
		{
			name:     "with spaces",
			input:    "1d 2h 30m",
			expected: 24*time.Hour + 2*time.Hour + 30*time.Minute,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:        "invalid format",
			input:       "invalid",
			expectError: true,
		},
		{
			name:     "seconds only",
			input:    "45s",
			expected: 45 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseLongDuration(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSplitStringToLengthByLine(t *testing.T) {
	t.Run("short input returns single part", func(t *testing.T) {
		parts := SplitStringToLengthByLine("hello\nworld", 2000)
		require.Len(t, parts, 1)
		assert.Contains(t, parts[0], "hello")
		assert.Contains(t, parts[0], "world")
	})

	t.Run("no part exceeds length", func(t *testing.T) {
		var lines []string
		for range 100 {
			lines = append(lines, strings.Repeat("x", 40))
		}
		input := strings.Join(lines, "\n")

		parts := SplitStringToLengthByLine(input, 200)
		require.Greater(t, len(parts), 1)
		for i, part := range parts {
			assert.LessOrEqual(t, len(part), 200, "part %d exceeds length", i)
		}
	})

	t.Run("newline added on flush boundary does not exceed length", func(t *testing.T) {
		// First line fills the buffer to 1960 chars (1959 + newline); the
		// second line of 40 chars makes exactly 2000 before the trailing
		// newline is appended, which must not push the part to 2001.
		input := strings.Repeat("a", 1959) + "\n" + strings.Repeat("b", 40)

		parts := SplitStringToLengthByLine(input, 2000)
		for i, part := range parts {
			assert.LessOrEqual(t, len(part), 2000, "part %d exceeds length", i)
		}
	})

	t.Run("no content is lost", func(t *testing.T) {
		var lines []string
		for i := range 50 {
			lines = append(lines, fmt.Sprintf("line-%d", i))
		}
		input := strings.Join(lines, "\n")

		parts := SplitStringToLengthByLine(input, 100)
		joined := strings.Join(parts, "")
		for _, line := range lines {
			assert.Contains(t, joined, line)
		}
	})
}

func TestFormatFloatUpToPrec(t *testing.T) {
	tests := []struct {
		name     string
		num      float64
		prec     int
		expected string
	}{
		{
			name:     "no decimal needed",
			num:      5.0,
			prec:     2,
			expected: "5",
		},
		{
			name:     "with decimal",
			num:      5.25,
			prec:     2,
			expected: "5.25",
		},
		{
			name:     "trailing zeros removed",
			num:      5.200,
			prec:     3,
			expected: "5.2",
		},
		{
			name:     "precision limited",
			num:      5.123456,
			prec:     2,
			expected: "5.12",
		},
		{
			name:     "zero",
			num:      0.0,
			prec:     2,
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatFloatUpToPrec(tt.num, tt.prec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

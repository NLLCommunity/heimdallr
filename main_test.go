package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected int
	}{
		{
			name:     "debug level",
			level:    "debug",
			expected: -4, // slog.LevelDebug
		},
		{
			name:     "info level",
			level:    "info",
			expected: 0, // slog.LevelInfo
		},
		{
			name:     "warn level",
			level:    "warn",
			expected: 4, // slog.LevelWarn
		},
		{
			name:     "error level",
			level:    "error",
			expected: 8, // slog.LevelError
		},
		{
			name:     "unknown level defaults to info",
			level:    "unknown",
			expected: 0, // slog.LevelInfo
		},
		{
			name:     "uppercase level",
			level:    "DEBUG",
			expected: -4, // slog.LevelDebug
		},
		{
			name:     "mixed case level",
			level:    "WaRn",
			expected: 4, // slog.LevelWarn
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLogLevel(tt.level)
			assert.Equal(t, tt.expected, int(result))
		})
	}
}

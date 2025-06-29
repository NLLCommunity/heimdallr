package utils

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDiscordTime(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC)
	dt := DiscordTime{fixedTime}
	expectedUnix := fixedTime.Unix()

	tests := []struct {
		name   string
		method func() string
		format string
	}{
		{
			name:   "ToRelative",
			method: dt.ToRelative,
			format: "R",
		},
		{
			name:   "ToShortTime",
			method: dt.ToShortTime,
			format: "t",
		},
		{
			name:   "ToLongTime",
			method: dt.ToLongTime,
			format: "T",
		},
		{
			name:   "ToShortDate",
			method: dt.ToShortDate,
			format: "d",
		},
		{
			name:   "ToLongDate",
			method: dt.ToLongDate,
			format: "D",
		},
		{
			name:   "ToShortDateTime",
			method: dt.ToShortDateTime,
			format: "f",
		},
		{
			name:   "ToLongDateTime",
			method: dt.ToLongDateTime,
			format: "F",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method()
			expected := fmt.Sprintf("<t:%d:%s>", expectedUnix, tt.format)
			assert.Equal(t, expected, result)
		})
	}
}

func TestDiscordTimeWithDifferentTimezones(t *testing.T) {
	// Test that the time is converted to UTC regardless of input timezone.
	loc, err := time.LoadLocation("America/New_York")
	assert.NoError(t, err)

	// Same moment in time, different timezone.
	easternTime := time.Date(2024, 1, 15, 9, 30, 45, 0, loc) // EST (UTC-5)
	utcTime := time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC)

	dtEastern := DiscordTime{easternTime}
	dtUTC := DiscordTime{utcTime}

	// Both should produce the same Discord timestamp since they represent the same moment.
	assert.Equal(t, dtUTC.ToRelative(), dtEastern.ToRelative())
	assert.Equal(t, dtUTC.ToShortTime(), dtEastern.ToShortTime())
	assert.Equal(t, dtUTC.ToLongTime(), dtEastern.ToLongTime())
}

func TestDiscordTimeZeroValue(t *testing.T) {
	dt := DiscordTime{time.Time{}}

	// Zero time should produce a specific timestamp.
	expected := "<t:-62135596800:R>" // Unix timestamp for zero time.
	assert.Equal(t, expected, dt.ToRelative())
}

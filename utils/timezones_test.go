package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTimezoneAcceptsCanonicalIANANames(t *testing.T) {
	for _, name := range []string{"UTC", "Europe/Oslo", "America/New_York"} {
		location, err := ValidateTimezone(name)
		require.NoError(t, err, name)
		assert.Equal(t, name, location.String())
	}

	for _, name := range []string{"", "UTC+02:00", "CET", "Local", "europe/oslo"} {
		_, err := ValidateTimezone(name)
		assert.Error(t, err, name)
	}
}

func TestTimezoneNamesReturnsAnImmutableDeterministicCatalog(t *testing.T) {
	first := TimezoneNames()
	second := TimezoneNames()
	require.NotEmpty(t, first)
	assert.Equal(t, first, second)

	first[0] = "changed"
	assert.NotEqual(t, first[0], TimezoneNames()[0])
}

func TestFilterTimezonesIsCaseInsensitiveAndLimited(t *testing.T) {
	assert.Equal(t, []string{"Europe/Oslo"}, FilterTimezones("oSlO", 25))

	results := FilterTimezones("america/new", 25)
	require.NotEmpty(t, results)
	assert.Equal(t, "America/New_York", results[0])
	assert.LessOrEqual(t, len(FilterTimezones("a", 25)), 25)
	assert.Empty(t, FilterTimezones("a", 0))
}

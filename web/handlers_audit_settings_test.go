package web

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uintPtr(v uint) *uint { return &v }

func TestParseRetentionField(t *testing.T) {
	const configKey = "audit_log.test_retention_days"

	cases := []struct {
		name    string
		raw     string
		ceiling int // value set on viper for configKey
		want    *uint
		wantErr bool
	}{
		{
			name:    "empty string returns nil (use default)",
			raw:     "",
			ceiling: 30,
			want:    nil,
		},
		{
			name:    "whitespace-only returns nil",
			raw:     "   ",
			ceiling: 30,
			want:    nil,
		},
		{
			name:    "valid value under ceiling is accepted",
			raw:     "14",
			ceiling: 30,
			want:    uintPtr(14),
		},
		{
			name:    "value equal to ceiling is accepted",
			raw:     "30",
			ceiling: 30,
			want:    uintPtr(30),
		},
		{
			name:    "value above ceiling is rejected",
			raw:     "100",
			ceiling: 30,
			wantErr: true,
		},
		{
			name:    "zero with finite ceiling is rejected",
			raw:     "0",
			ceiling: 30,
			wantErr: true,
		},
		{
			name:    "zero with zero ceiling normalizes to nil (no stale override)",
			raw:     "0",
			ceiling: 0,
			want:    nil,
		},
		{
			name:    "non-zero with zero ceiling is accepted",
			raw:     "7",
			ceiling: 0,
			want:    uintPtr(7),
		},
		{
			name:    "non-numeric is rejected",
			raw:     "abc",
			ceiling: 30,
			wantErr: true,
		},
		{
			name:    "negative is rejected (ParseUint rejects sign)",
			raw:     "-1",
			ceiling: 30,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			viper.Set(configKey, c.ceiling)
			t.Cleanup(func() { viper.Set(configKey, 0) })

			got, err := parseRetentionField(c.raw, configKey, "Test")
			if c.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if c.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *c.want, *got)
			}
		})
	}
}

func TestRetentionOverrideDisplay(t *testing.T) {
	cases := []struct {
		name     string
		override *uint
		ceiling  uint
		want     string
	}{
		{
			name:     "nil override renders as empty (use default)",
			override: nil,
			ceiling:  30,
			want:     "",
		},
		{
			name:     "nil override with zero ceiling renders as empty",
			override: nil,
			ceiling:  0,
			want:     "",
		},
		{
			name:     "non-zero override renders as the value",
			override: uintPtr(14),
			ceiling:  30,
			want:     "14",
		},
		{
			name:     "stored zero with finite ceiling renders as empty — legacy row whose value is now invalid",
			override: uintPtr(0),
			ceiling:  30,
			want:     "",
		},
		{
			name:     "stored zero with zero ceiling renders literal — value is still valid (forever)",
			override: uintPtr(0),
			ceiling:  0,
			want:     "0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := retentionOverrideDisplay(c.override, c.ceiling)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestFormatSettingsUpdate_SkipsMetadata(t *testing.T) {
	// formatSettingsUpdate calls formatSettingsValue, which for plain
	// scalar values (no channel/role suffix) doesn't touch the disgo
	// cache — we can pass a nil client + zero guildID and still exercise
	// the metadata-filter path. The "Changed values" section body should
	// contain only the real-setting keys, not actor_username/section.
	details := map[string]any{
		"section":        "audit_log",
		"actor_username": "@admin",
		"enabled":        true,
		"half_life_days": float64(14),
	}

	summary, sections := formatSettingsUpdate(nil, 0, details)
	assert.Equal(t, "Audit log", summary)
	require.Len(t, sections, 1)
	body := sections[0].Body

	// Must contain the real settings.
	assert.Contains(t, body, "Enabled:")
	assert.Contains(t, body, "Half life days:")
	// Must NOT contain attribution metadata.
	assert.NotContains(t, body, "Actor username")
	assert.NotContains(t, body, "Section:")
}

func TestFormatSettingsUpdate_OnlyMetadata_ReturnsNoSections(t *testing.T) {
	// A degenerate payload that only carries metadata (no settings were
	// actually included) should still render the section header but
	// produce no "Changed values" disclosure.
	details := map[string]any{
		"section":        "modmail",
		"actor_username": "@admin",
	}

	summary, sections := formatSettingsUpdate(nil, 0, details)
	assert.Equal(t, "Modmail", summary)
	assert.Nil(t, sections)
}

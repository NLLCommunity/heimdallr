package model

import (
	"fmt"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBirthdaySettingsDefaults(t *testing.T) {
	settings := GuildSettings{}
	ResetBirthdaySettings(&settings)

	assert.False(t, settings.BirthdayEnabled)
	assert.Zero(t, settings.BirthdayChannel)
	assert.Equal(t, "UTC", settings.BirthdayTimezone)
	assert.Equal(t, 10, settings.BirthdayHour)
	assert.Zero(t, settings.BirthdayMinute)
	assert.Equal(t, "Happy birthday, {{User.Mention}}! 🎉", settings.BirthdayMessage)
	assert.False(t, settings.BirthdayMessageV2)
	assert.Empty(t, settings.BirthdayMessageV2Json)
}

func TestParseBirthdayTimeRequiresQuarterHourHHMM(t *testing.T) {
	for hour := range 24 {
		for _, minute := range []int{0, 15, 30, 45} {
			raw := fmt.Sprintf("%02d:%02d", hour, minute)
			gotHour, gotMinute, err := ParseBirthdayTime(raw)
			require.NoError(t, err, raw)
			assert.Equal(t, hour, gotHour, raw)
			assert.Equal(t, minute, gotMinute, raw)
			assert.Equal(t, raw, FormatBirthdayTime(gotHour, gotMinute))
		}
	}

	for _, raw := range []string{"", "1:00", "10:01", "10:60", "24:00", "10:00Z", " 10:00"} {
		_, _, err := ParseBirthdayTime(raw)
		assert.Error(t, err, raw)
	}
}

func TestValidateBirthdaySettings(t *testing.T) {
	settings := GuildSettings{}
	ResetBirthdaySettings(&settings)
	assert.NoError(t, ValidateBirthdaySettings(&settings))

	settings.BirthdayEnabled = true
	assert.Error(t, ValidateBirthdaySettings(&settings))

	settings.BirthdayChannel = 123
	assert.NoError(t, ValidateBirthdaySettings(&settings))

	settings.BirthdayMinute = 1
	assert.Error(t, ValidateBirthdaySettings(&settings))
	settings.BirthdayMinute = 0

	settings.BirthdayTimezone = "UTC+02:00"
	assert.Error(t, ValidateBirthdaySettings(&settings))
	settings.BirthdayTimezone = "UTC"

	settings.BirthdayMessage = "{{#broken}}"
	assert.Error(t, ValidateBirthdaySettings(&settings))
	settings.BirthdayMessage = DefaultBirthdayMessage

	settings.BirthdayMessageV2 = true
	settings.BirthdayMessageV2Json = ""
	assert.Error(t, ValidateBirthdaySettings(&settings))
}

func (suite *ModelTestSuite) TestNewGuildSettingsUseBirthdayDefaults() {
	settings, err := GetGuildSettings(123)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), DefaultBirthdayTimezone, settings.BirthdayTimezone)
	assert.Equal(suite.T(), DefaultBirthdayHour, settings.BirthdayHour)
	assert.Equal(suite.T(), DefaultBirthdayMinute, settings.BirthdayMinute)
	assert.Equal(suite.T(), DefaultBirthdayMessage, settings.BirthdayMessage)
}

func (suite *ModelTestSuite) TestGetBirthdayAnnouncementGuildsReturnsOnlyEnabledConfiguredRows() {
	rows := []GuildSettings{
		{GuildID: 10, BirthdayEnabled: true, BirthdayChannel: 100},
		{GuildID: 11, BirthdayEnabled: false, BirthdayChannel: 101},
		{GuildID: 12, BirthdayEnabled: true, BirthdayChannel: 0},
		{GuildID: 13, BirthdayEnabled: true, BirthdayChannel: 103},
	}
	for i := range rows {
		ResetBirthdaySettings(&rows[i])
		rows[i].GuildID = snowflake.ID(10 + i)
		rows[i].BirthdayEnabled = i != 1
		if i != 2 {
			rows[i].BirthdayChannel = snowflake.ID(100 + i)
		}
		require.NoError(suite.T(), suite.db.Create(&rows[i]).Error)
	}

	settings, err := GetBirthdayAnnouncementGuilds()
	require.NoError(suite.T(), err)
	ids := make([]snowflake.ID, 0, len(settings))
	for _, row := range settings {
		ids = append(ids, row.GuildID)
	}
	assert.ElementsMatch(suite.T(), []snowflake.ID{10, 13}, ids)
}

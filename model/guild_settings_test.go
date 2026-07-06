package model

import (
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *ModelTestSuite) TestGetPostsModRoles_BatchLookup() {
	withRole := GuildSettings{GuildID: 1, PostsModRoleID: 100}
	withoutRole := GuildSettings{GuildID: 2, PostsModRoleID: 0}
	require.NoError(suite.T(), DB.Create(&withRole).Error)
	require.NoError(suite.T(), DB.Create(&withoutRole).Error)

	// Guild 3 has no settings row at all.
	roles, err := GetPostsModRoles([]snowflake.ID{1, 2, 3})
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), map[snowflake.ID]snowflake.ID{1: 100}, roles,
		"only guilds with a configured role should be present")

	// Read-only: the lookup must not have created a row for guild 3
	// (unlike GetGuildSettings, which is a FirstOrCreate).
	var count int64
	require.NoError(suite.T(), DB.Model(&GuildSettings{}).Where("guild_id = ?", 3).Count(&count).Error)
	assert.Zero(suite.T(), count, "batch lookup must not insert settings rows")
}

func (suite *ModelTestSuite) TestGetPostsModRoles_EmptyInput() {
	roles, err := GetPostsModRoles(nil)
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), roles)
}

// UpdateGuildSettingsColumns must write only the named columns. This is the
// concurrent-section-save scenario: two handlers each load the row, then save
// different sections. With a whole-row Save the second writer reverts the
// first; with column-scoped updates both survive.
func (suite *ModelTestSuite) TestUpdateGuildSettingsColumns_NoClobber() {
	t := suite.T()
	guildID := snowflake.ID(123456789)

	_, err := GetGuildSettings(guildID)
	require.NoError(t, err)

	// Two independent snapshots, as two concurrent requests would load.
	snapA, err := GetGuildSettings(guildID)
	require.NoError(t, err)
	snapB, err := GetGuildSettings(guildID)
	require.NoError(t, err)

	// Handler A saves the mod-channel section.
	snapA.ModeratorChannel = snowflake.ID(444555666)
	require.NoError(t, UpdateGuildSettingsColumns(snapA, "ModeratorChannel"))

	// Handler B, working from a snapshot taken before A committed, saves the
	// anti-spam section. It must not revert A's ModeratorChannel.
	snapB.AntiSpamCount = 9
	require.NoError(t, UpdateGuildSettingsColumns(snapB, "AntiSpamEnabled", "AntiSpamCount", "AntiSpamCooldownSeconds"))

	got, err := GetGuildSettings(guildID)
	require.NoError(t, err)
	assert.Equal(t, snowflake.ID(444555666), got.ModeratorChannel, "A's mod channel must survive B's save")
	assert.Equal(t, 9, got.AntiSpamCount, "B's anti-spam count must be persisted")
}

// Selected columns must be written even when they hold the zero value, so a
// toggle can actually be turned off and a nil retention override clears to NULL.
func (suite *ModelTestSuite) TestUpdateGuildSettingsColumns_WritesZeroValues() {
	t := suite.T()
	guildID := snowflake.ID(222333444)

	settings, err := GetGuildSettings(guildID)
	require.NoError(t, err)
	threeDays := uint(3)
	settings.AntiSpamEnabled = true
	settings.AuditMessageRetentionDays = &threeDays
	require.NoError(t, UpdateGuildSettingsColumns(settings, "AntiSpamEnabled", "AuditMessageRetentionDays"))

	// Now flip the bool back to false and clear the pointer to nil.
	settings.AntiSpamEnabled = false
	settings.AuditMessageRetentionDays = nil
	require.NoError(t, UpdateGuildSettingsColumns(settings, "AntiSpamEnabled", "AuditMessageRetentionDays"))

	got, err := GetGuildSettings(guildID)
	require.NoError(t, err)
	assert.False(t, got.AntiSpamEnabled, "zero-value bool must be written")
	assert.Nil(t, got.AuditMessageRetentionDays, "nil pointer must clear the column to NULL")
}

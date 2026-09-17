package model

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func (suite *ModelTestSuite) TestStarboardMigrationAndIsolation() {
	t := suite.T()
	require.True(t, suite.db.Migrator().HasTable(&Starboard{}))
	board := Starboard{GuildID: 10, ChannelID: 20, Name: "Stars", Emoji: "⭐", Threshold: 4}
	require.NoError(t, suite.db.Create(&board).Error)
	duplicate := Starboard{GuildID: 10, ChannelID: 21, Name: "Stars", Emoji: "⭐", Threshold: 4}
	require.Error(t, suite.db.Create(&duplicate).Error)
	other := Starboard{GuildID: 11, ChannelID: 22, Name: "Stars", Emoji: "⭐", Threshold: 4}
	require.NoError(t, suite.db.Create(&other).Error)
	entry := StarboardEntry{GuildID: 10, BoardID: board.ID, ChannelID: 30, MessageID: 40}
	require.NoError(t, suite.db.Create(&entry).Error)
	entry.ID = 0
	require.Error(t, suite.db.Create(&entry).Error)
}

func TestStarboardDefaultsDisabled(t *testing.T) {
	require.False(t, GuildSettings{}.StarboardEnabled)
	require.False(t, Starboard{}.Enabled)
}

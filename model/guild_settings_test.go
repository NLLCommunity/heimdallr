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

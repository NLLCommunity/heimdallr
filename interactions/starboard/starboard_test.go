package starboard

import (
	"encoding/json"
	"testing"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/model"
)

func TestParseMessageReference(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		channel     snowflake.ID
		guild       snowflake.ID
		wantChannel snowflake.ID
		wantMessage snowflake.ID
		wantErr     string
	}{
		{name: "discord link", raw: "https://discord.com/channels/100/200/300", guild: 100, wantChannel: 200, wantMessage: 300},
		{name: "canary link", raw: "https://canary.discord.com/channels/100/200/300", guild: 100, wantChannel: 200, wantMessage: 300},
		{name: "message ID with channel", raw: "300", channel: 200, guild: 100, wantChannel: 200, wantMessage: 300},
		{name: "message ID requires channel", raw: "300", guild: 100, wantErr: "channel"},
		{name: "zero message ID", raw: "0", channel: 200, guild: 100, wantErr: "message"},
		{name: "zero link channel", raw: "https://discord.com/channels/100/0/300", guild: 100, wantErr: "message"},
		{name: "wrong guild link", raw: "https://discord.com/channels/999/200/300", guild: 100, wantErr: "guild"},
		{name: "foreign host", raw: "https://example.com/channels/100/200/300", guild: 100, wantErr: "link"},
		{name: "malformed", raw: "not-a-message", guild: 100, wantErr: "message"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			channel, message, err := parseMessageReference(tc.raw, tc.channel, tc.guild)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantChannel, channel)
			assert.Equal(t, tc.wantMessage, message)
		})
	}
}

func TestModerationPermissionUsesBoardDestination(t *testing.T) {
	caches := moderationCaches(t)
	member := discord.Member{GuildID: 100, User: discord.User{ID: 500}, RoleIDs: []snowflake.ID{501}}
	board := model.Starboard{GuildID: 100, ChannelID: 200}

	assert.True(t, canModerateBoard(caches, member, board), "destination overwrite grants Manage Messages")
	assert.False(t, canModerateGuild(caches, member), "channel overwrite must not grant server-wide moderation")

	board.ChannelID = 201
	assert.False(t, canModerateBoard(caches, member, board), "permission in the invoking channel must not authorize another destination")
}

func TestAdministratorCanModerateBoardAndGuild(t *testing.T) {
	caches := moderationCaches(t)
	member := discord.Member{GuildID: 100, User: discord.User{ID: 600}, RoleIDs: []snowflake.ID{601}}
	board := model.Starboard{GuildID: 100, ChannelID: 201}

	assert.True(t, canModerateBoard(caches, member, board))
	assert.True(t, canModerateGuild(caches, member))

	board.ChannelID = 9999
	assert.True(t, canModerateBoard(caches, member, board), "administrator permission does not depend on a warm destination cache")
}

func TestCachedGuildChannelRejectsAnotherGuild(t *testing.T) {
	caches := moderationCaches(t)
	var foreign discord.GuildTextChannel
	require.NoError(t, json.Unmarshal([]byte(`{"id":"300","guild_id":"999","type":0,"name":"foreign"}`), &foreign))
	caches.AddChannel(foreign)

	_, ok := cachedGuildChannel(caches, 100, 300)
	assert.False(t, ok)

	channel, ok := cachedGuildChannel(caches, 100, 200)
	require.True(t, ok)
	assert.Equal(t, snowflake.ID(100), channel.GuildID())
}

func TestCommandDefaultsToManageMessagesAndGuildContext(t *testing.T) {
	built := Command.Build().(discord.SlashCommandCreate)
	require.NotNil(t, built.DefaultMemberPermissions.Value)
	assert.Equal(t, discord.PermissionManageMessages, *built.DefaultMemberPermissions.Value)
	assert.Equal(t, []discord.InteractionContextType{discord.InteractionContextTypeGuild}, built.Contexts)
	assert.Len(t, built.Options, 6)
}

func moderationCaches(t *testing.T) cache.Caches {
	t.Helper()
	caches := cache.New(cache.WithCaches(cache.FlagGuilds, cache.FlagChannels, cache.FlagRoles))
	caches.AddGuild(discord.Guild{ID: 100, Name: "Guild", OwnerID: 900})
	caches.AddRole(discord.Role{ID: 100, GuildID: 100, Name: "@everyone"})
	caches.AddRole(discord.Role{ID: 501, GuildID: 100, Name: "Channel mod"})
	caches.AddRole(discord.Role{ID: 601, GuildID: 100, Name: "Admin", Permissions: discord.PermissionAdministrator})

	var allowed discord.GuildTextChannel
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"200","guild_id":"100","type":0,"name":"starboard",
		"permission_overwrites":[{"id":"501","type":0,"allow":"8192","deny":"0"}]
	}`), &allowed))
	caches.AddChannel(allowed)
	var denied discord.GuildTextChannel
	require.NoError(t, json.Unmarshal([]byte(`{"id":"201","guild_id":"100","type":0,"name":"other"}`), &denied))
	caches.AddChannel(denied)
	return caches
}

package ban

import (
	"context"
	"net/http"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
)

func TestBanCommandRequiresBanMembersPermission(t *testing.T) {
	command := Ban.Build().(discord.SlashCommandCreate)

	require.Equal(t, omit.NewPtr(discord.PermissionBanMembers), command.DefaultMemberPermissions)
}

type recordingRESTClient struct {
	requests int
}

func (*recordingRESTClient) HTTPClient() *http.Client {
	return nil
}

func (*recordingRESTClient) RateLimiter() rest.RateLimiter {
	return nil
}

func (*recordingRESTClient) Close(context.Context) {}

func (c *recordingRESTClient) Do(
	*rest.CompiledEndpoint,
	any,
	any,
	...rest.RequestOpt,
) error {
	c.requests++
	return nil
}

func TestBanHandlerRejectsKickOnlyMemberBeforeRESTCall(t *testing.T) {
	previousDB := model.DB
	db, err := model.InitDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
		model.DB = previousDB
	})

	interaction, err := discord.UnmarshalInteraction([]byte(`{
		"id": "175928847299117063",
		"application_id": "123456789012345678",
		"type": 2,
		"token": "interaction-token",
		"version": 1,
		"guild_id": "200",
		"channel": {"id": "201", "type": 0, "name": "general"},
		"member": {
			"user": {"id": "100", "username": "moderator", "discriminator": "0"},
			"roles": [],
			"permissions": "2"
		},
		"data": {
			"id": "300",
			"name": "ban",
			"type": 1,
			"resolved": {
				"users": {
					"400": {"id": "400", "username": "target", "discriminator": "0"}
				}
			},
			"options": [
				{"name": "user", "type": 6, "value": "400"},
				{"name": "message", "type": 3, "value": "Moderation notice"}
			]
		}
	}`))
	require.NoError(t, err)

	recorder := &recordingRESTClient{}
	caches := cache.New(cache.WithCaches(cache.FlagGuilds))
	caches.AddGuild(discord.Guild{ID: 200, Name: "Test Guild"})
	client := &bot.Client{
		Rest:   rest.New(recorder),
		Caches: caches,
	}

	var response discord.MessageCreate
	responded := false
	event := &handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			GenericEvent:                  events.NewGenericEvent(client, 0, 0),
			ApplicationCommandInteraction: interaction.(discord.ApplicationCommandInteraction),
			Respond: func(responseType discord.InteractionResponseType, data discord.InteractionResponseData, _ ...rest.RequestOpt) error {
				require.Equal(t, discord.InteractionResponseTypeCreateMessage, responseType)
				response = data.(discord.MessageCreate)
				responded = true
				return nil
			},
		},
	}

	require.NoError(t, BanHandler(event))
	require.True(t, responded)
	require.Equal(t, "You need the Ban Members permission to ban a user.", response.Content)
	require.True(t, response.Flags.Has(discord.MessageFlagEphemeral))
	require.Zero(t, recorder.requests)
}

func TestBanHandlerWithoutGuildReturnsNoGuildError(t *testing.T) {
	interaction, err := discord.UnmarshalInteraction([]byte(`{
		"id": "175928847299117063",
		"application_id": "123456789012345678",
		"type": 2,
		"token": "interaction-token",
		"version": 1,
		"channel": {"id": "201", "type": 1},
		"user": {"id": "100", "username": "moderator", "discriminator": "0"},
		"data": {"id": "300", "name": "ban", "type": 1}
	}`))
	require.NoError(t, err)

	responded := false
	event := &handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			GenericEvent:                  events.NewGenericEvent(&bot.Client{Caches: cache.New()}, 0, 0),
			ApplicationCommandInteraction: interaction.(discord.ApplicationCommandInteraction),
			Respond: func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
				responded = true
				return nil
			},
		},
	}

	require.ErrorIs(t, BanHandler(event), interactions.ErrEventNoGuildID)
	require.False(t, responded)
}

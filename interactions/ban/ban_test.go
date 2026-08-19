package ban

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"
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

type banEventFixture struct {
	invokerID          snowflake.ID
	invokerPermissions discord.Permissions
	invokerRoleIDs     []snowflake.ID
	targetID           snowflake.ID
	targetPermissions  discord.Permissions
	targetRoleIDs      []snowflake.ID
	guildOwnerID       snowflake.ID
	roles              []discord.Role
	resolveTarget      bool
}

func eligibleBanEventFixture() banEventFixture {
	return banEventFixture{
		invokerID:          100,
		invokerPermissions: discord.PermissionBanMembers,
		invokerRoleIDs:     []snowflake.ID{101},
		targetID:           400,
		targetRoleIDs:      []snowflake.ID{401},
		guildOwnerID:       999,
		roles: []discord.Role{
			{ID: 101, GuildID: 200, Name: "Moderator", Position: 10},
			{ID: 401, GuildID: 200, Name: "Member", Position: 5},
		},
		resolveTarget: true,
	}
}

type recordedBanResponse struct {
	calls   int
	message discord.MessageCreate
}

func newBanEventWithFixture(
	t *testing.T,
	recorder *recordingRESTClient,
	fixture banEventFixture,
	message string,
) (*handler.CommandEvent, *recordedBanResponse) {
	t.Helper()

	invokerRoleIDs, err := json.Marshal(fixture.invokerRoleIDs)
	require.NoError(t, err)
	targetRoleIDs, err := json.Marshal(fixture.targetRoleIDs)
	require.NoError(t, err)

	resolvedMembers := "{}"
	if fixture.resolveTarget {
		resolvedMembers = fmt.Sprintf(
			`{"%d":{"roles":%s,"permissions":"%d"}}`,
			fixture.targetID,
			targetRoleIDs,
			fixture.targetPermissions,
		)
	}
	messageOption := ""
	if message != "" {
		messageOption = fmt.Sprintf(`,{"name":"message","type":3,"value":%q}`, message)
	}

	interaction, err := discord.UnmarshalInteraction([]byte(fmt.Sprintf(`{
		"id":"175928847299117063",
		"application_id":"123456789012345678",
		"type":2,
		"token":"interaction-token",
		"version":1,
		"guild_id":"200",
		"channel":{"id":"201","type":0,"name":"general"},
		"member":{
			"user":{"id":"%d","username":"moderator","discriminator":"0"},
			"roles":%s,
			"permissions":"%d"
		},
		"data":{
			"id":"300",
			"name":"ban",
			"type":1,
			"resolved":{
				"users":{"%d":{"id":"%d","username":"target","discriminator":"0"}},
				"members":%s
			},
			"options":[
				{"name":"user","type":6,"value":"%d"}%s
			]
		}
	}`,
		fixture.invokerID,
		invokerRoleIDs,
		fixture.invokerPermissions,
		fixture.targetID,
		fixture.targetID,
		resolvedMembers,
		fixture.targetID,
		messageOption,
	)))
	require.NoError(t, err)

	caches := cache.New(cache.WithCaches(cache.FlagGuilds, cache.FlagRoles))
	caches.AddGuild(discord.Guild{ID: 200, Name: "Test Guild", OwnerID: fixture.guildOwnerID})
	for _, role := range fixture.roles {
		caches.AddRole(role)
	}
	client := &bot.Client{Rest: rest.New(recorder), Caches: caches}
	response := &recordedBanResponse{}

	event := &handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			GenericEvent:                  events.NewGenericEvent(client, 0, 0),
			ApplicationCommandInteraction: interaction.(discord.ApplicationCommandInteraction),
			Respond: func(responseType discord.InteractionResponseType, data discord.InteractionResponseData, _ ...rest.RequestOpt) error {
				require.Equal(t, discord.InteractionResponseTypeCreateMessage, responseType)
				message, ok := data.(discord.MessageCreate)
				require.True(t, ok)
				response.calls++
				response.message = message
				return nil
			},
		},
	}
	return event, response
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

func TestBanHandlerRejectsIneligibleTargetsBeforeRESTCall(t *testing.T) {
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

	tests := []struct {
		name    string
		fixture banEventFixture
	}{
		{
			name: "unresolved target",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.resolveTarget = false
				return fixture
			}(),
		},
		{
			name: "self",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.targetID = fixture.invokerID
				return fixture
			}(),
		},
		{
			name: "guild owner",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.targetID = fixture.guildOwnerID
				return fixture
			}(),
		},
		{
			name: "administrator permission",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.targetPermissions = discord.PermissionAdministrator
				return fixture
			}(),
		},
		{
			name: "administrator role",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.targetRoleIDs = []snowflake.ID{402}
				fixture.roles = append(fixture.roles, discord.Role{
					ID:          402,
					GuildID:     200,
					Name:        "Administrator",
					Position:    5,
					Permissions: discord.PermissionAdministrator,
				})
				return fixture
			}(),
		},
		{
			name: "equal highest role",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.roles[1].Position = 10
				return fixture
			}(),
		},
		{
			name: "higher role",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.roles[1].Position = 11
				return fixture
			}(),
		},
		{
			name: "highest of multiple target roles",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.targetRoleIDs = []snowflake.ID{401, 402}
				fixture.roles = append(fixture.roles, discord.Role{
					ID: 402, GuildID: 200, Name: "Higher", Position: 11,
				})
				return fixture
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingRESTClient{}
			event, response := newBanEventWithFixture(t, recorder, test.fixture, "Moderation notice")

			require.NoError(t, BanHandler(event))

			require.Equal(t, 1, response.calls)
			require.Equal(t, "You cannot ban this user.", response.message.Content)
			require.True(t, response.message.Flags.Has(discord.MessageFlagEphemeral))
			require.Zero(t, recorder.requests)
		})
	}
}

func TestBanHandlerAllowsEligibleTargets(t *testing.T) {
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

	tests := []struct {
		name    string
		fixture banEventFixture
	}{
		{name: "lower role", fixture: eligibleBanEventFixture()},
		{
			name: "highest of multiple invoker roles",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.invokerRoleIDs = []snowflake.ID{102, 101}
				fixture.roles = append(fixture.roles, discord.Role{
					ID: 102, GuildID: 200, Name: "Lower", Position: 1,
				})
				return fixture
			}(),
		},
		{
			name: "guild owner bypasses role hierarchy",
			fixture: func() banEventFixture {
				fixture := eligibleBanEventFixture()
				fixture.invokerID = fixture.guildOwnerID
				fixture.roles[1].Position = 11
				return fixture
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingRESTClient{}
			event, response := newBanEventWithFixture(t, recorder, test.fixture, "")

			require.NoError(t, BanHandler(event))

			require.Equal(t, 1, response.calls)
			require.Equal(t, "User was banned.", response.message.Content)
			require.True(t, response.message.Flags.Has(discord.MessageFlagEphemeral))
			require.Equal(t, 1, recorder.requests)
		})
	}
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

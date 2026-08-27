package birthday

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/NLLCommunity/heimdallr/model"
)

func TestBirthdayCommandDefinition(t *testing.T) {
	command := Birthday.Build().(discord.SlashCommandCreate)
	require.Equal(t, []discord.InteractionContextType{discord.InteractionContextTypeGuild}, command.Contexts)
	require.Nil(t, command.DefaultMemberPermissions.Value)
	require.Len(t, command.Options, 3)

	set := command.Options[0].(discord.ApplicationCommandOptionSubCommand)
	assert.Equal(t, "set", set.Name)
	require.Len(t, set.Options, 3)
	assert.Equal(t, "day", set.Options[0].OptionName())
	assert.Equal(t, "month", set.Options[1].OptionName())
	assert.Equal(t, "year", set.Options[2].OptionName())
	assert.True(t, set.Options[0].(discord.ApplicationCommandOptionInt).Required)
	assert.True(t, set.Options[1].(discord.ApplicationCommandOptionInt).Required)
	assert.False(t, set.Options[2].(discord.ApplicationCommandOptionInt).Required)

	assert.Equal(t, "status", command.Options[1].OptionName())
	assert.Equal(t, "remove", command.Options[2].OptionName())
}

func TestBuildLeapDayPromptIsPrivateAndEncodesChoice(t *testing.T) {
	message, err := buildLeapDayPrompt(snowflake.ID(123), nil)
	require.NoError(t, err)
	assert.NotZero(t, message.Flags&discord.MessageFlagEphemeral)
	require.NotNil(t, message.AllowedMentions)
	assert.Empty(t, message.AllowedMentions.Users)
	assert.ElementsMatch(t, []string{
		"/birthday/leap/123/0/february_28",
		"/birthday/leap/123/0/march_1",
	}, birthdayComponentCustomIDs(message))

	year := 1990
	message, err = buildLeapDayPrompt(snowflake.ID(123), &year)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"/birthday/leap/123/1990/february_28",
		"/birthday/leap/123/1990/march_1",
	}, birthdayComponentCustomIDs(message))
}

func birthdayComponentCustomIDs(message discord.MessageCreate) []string {
	var ids []string
	for _, layout := range message.Components {
		row, ok := layout.(discord.ActionRowComponent)
		if !ok {
			continue
		}
		for _, component := range row.Components {
			button, ok := component.(discord.ButtonComponent)
			if ok {
				ids = append(ids, button.CustomID)
			}
		}
	}
	return ids
}

func TestBirthdayInteractionsRegisterLeapDayRoute(t *testing.T) {
	router := handler.New()
	Interactions(router)
	require.True(t, router.Match(
		"/birthday/leap/123/0/february_28",
		discord.InteractionTypeComponent,
		int(discord.ComponentTypeButton),
	))
}

func TestBirthdaySetHandlerSavesOrdinaryDatePrivately(t *testing.T) {
	setupBirthdayDatabase(t)
	event, response := newBirthdayCommandEvent(t, 10, 20, "set", `
		{"name":"day","type":4,"value":26},
		{"name":"month","type":4,"value":8},
		{"name":"year","type":4,"value":1990}
	`)

	require.NoError(t, BirthdaySetHandler(event))
	requireEphemeralBirthdayResponse(t, response)

	saved, err := model.GetBirthday(10, 20)
	require.NoError(t, err)
	assert.Equal(t, 26, saved.Day)
	assert.Equal(t, 8, int(saved.Month))
	require.NotNil(t, saved.BirthYear)
	assert.Equal(t, 1990, *saved.BirthYear)
}

func TestBirthdayLeapDayChoiceRequiresOwnerAndThenPersists(t *testing.T) {
	setupBirthdayDatabase(t)
	event, response := newBirthdayCommandEvent(t, 10, 20, "set", `
		{"name":"day","type":4,"value":29},
		{"name":"month","type":4,"value":2}
	`)
	require.NoError(t, BirthdaySetHandler(event))
	requireEphemeralBirthdayResponse(t, response)
	assert.Len(t, birthdayComponentCustomIDs(response.message), 2)
	_, err := model.GetBirthday(10, 20)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	unauthorized, unauthorizedResponse := newBirthdayComponentEvent(t, 10, 21, map[string]string{
		"userID": "20",
		"year":   "0",
		"policy": "february_28",
	})
	require.NoError(t, BirthdayLeapDayHandler(unauthorized))
	requireEphemeralBirthdayResponse(t, unauthorizedResponse)
	assert.Contains(t, strings.ToLower(unauthorizedResponse.message.Content), "your own")
	_, err = model.GetBirthday(10, 20)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	authorized, authorizedResponse := newBirthdayComponentEvent(t, 10, 20, map[string]string{
		"userID": "20",
		"year":   "0",
		"policy": "february_28",
	})
	require.NoError(t, BirthdayLeapDayHandler(authorized))
	require.Equal(t, 1, authorizedResponse.calls)
	assert.Equal(t, discord.InteractionResponseTypeUpdateMessage, authorizedResponse.responseType)
	require.NotNil(t, authorizedResponse.update.Content)
	assert.Equal(t, "Your birthday has been saved.", *authorizedResponse.update.Content)
	require.NotNil(t, authorizedResponse.update.Components)
	assert.Empty(t, *authorizedResponse.update.Components)
	saved, err := model.GetBirthday(10, 20)
	require.NoError(t, err)
	assert.Nil(t, saved.BirthYear)
	assert.Equal(t, model.LeapDayFebruary28, saved.LeapDayPolicy)
}

func TestBirthdayStatusShowsOnlyInvokingUsersBirthday(t *testing.T) {
	setupBirthdayDatabase(t)
	_, err := model.SetBirthday(10, 20, 26, 8, nil, "", 2026)
	require.NoError(t, err)
	_, err = model.SetBirthday(10, 21, 14, 2, nil, "", 2026)
	require.NoError(t, err)

	event, response := newBirthdayCommandEvent(t, 10, 20, "status", "")
	require.NoError(t, BirthdayStatusHandler(event))
	requireEphemeralBirthdayResponse(t, response)
	assert.Contains(t, response.message.Content, "26 August")
	assert.NotContains(t, response.message.Content, "14 February")
}

func TestBirthdayRemoveHandlerDeletesCompleteRecord(t *testing.T) {
	setupBirthdayDatabase(t)
	year := 1992
	_, err := model.SetBirthday(10, 20, 29, 2, &year, model.LeapDayMarch1, 2026)
	require.NoError(t, err)

	event, response := newBirthdayCommandEvent(t, 10, 20, "remove", "")
	require.NoError(t, BirthdayRemoveHandler(event))
	requireEphemeralBirthdayResponse(t, response)
	_, err = model.GetBirthday(10, 20)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

type birthdayResponse struct {
	calls        int
	responseType discord.InteractionResponseType
	message      discord.MessageCreate
	update       discord.MessageUpdate
}

func setupBirthdayDatabase(t *testing.T) {
	t.Helper()
	original := model.DB
	db, err := model.InitDB(filepath.Join(t.TempDir(), "birthdays.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = original
	})
}

func newBirthdayCommandEvent(
	t *testing.T,
	guildID snowflake.ID,
	userID snowflake.ID,
	subcommand string,
	options string,
) (*handler.CommandEvent, *birthdayResponse) {
	t.Helper()
	if strings.TrimSpace(options) != "" {
		options = ",\"options\":[" + options + "]"
	}
	interaction, err := discord.UnmarshalInteraction([]byte(fmt.Sprintf(`{
		"id":"1","application_id":"2","type":2,"token":"token","version":1,
		"guild_id":"%d","channel":{"id":"30","type":0,"name":"general"},
		"member":{"user":{"id":"%d","username":"user","discriminator":"0"},"roles":[],"permissions":"0"},
		"data":{"id":"3","name":"birthday","type":1,"options":[
			{"name":%q,"type":1%s}
		]}
	}`, guildID, userID, subcommand, options)))
	require.NoError(t, err)

	response := &birthdayResponse{}
	client := &bot.Client{Caches: cache.New()}
	return &handler.CommandEvent{ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
		GenericEvent:                  events.NewGenericEvent(client, 0, 0),
		ApplicationCommandInteraction: interaction.(discord.ApplicationCommandInteraction),
		Respond:                       birthdayResponder(t, response),
	}}, response
}

func newBirthdayComponentEvent(
	t *testing.T,
	guildID snowflake.ID,
	userID snowflake.ID,
	vars map[string]string,
) (*handler.ComponentEvent, *birthdayResponse) {
	t.Helper()
	customID := fmt.Sprintf("/birthday/leap/%s/%s/%s", vars["userID"], vars["year"], vars["policy"])
	interaction, err := discord.UnmarshalInteraction([]byte(fmt.Sprintf(`{
		"id":"1","application_id":"2","type":3,"token":"token","version":1,
		"guild_id":"%d","channel":{"id":"30","type":0,"name":"general"},
		"member":{"user":{"id":"%d","username":"user","discriminator":"0"},"roles":[],"permissions":"0"},
		"data":{"component_type":2,"custom_id":%q},
		"message":{"id":"4","channel_id":"30","author":{"id":"2","username":"bot","discriminator":"0"},"content":"choice"}
	}`, guildID, userID, customID)))
	require.NoError(t, err)

	response := &birthdayResponse{}
	client := &bot.Client{Caches: cache.New()}
	return &handler.ComponentEvent{
		ComponentInteractionCreate: &events.ComponentInteractionCreate{
			GenericEvent:         events.NewGenericEvent(client, 0, 0),
			ComponentInteraction: interaction.(discord.ComponentInteraction),
			Respond:              birthdayResponder(t, response),
		},
		Vars: vars,
	}, response
}

func birthdayResponder(t *testing.T, response *birthdayResponse) events.InteractionResponderFunc {
	t.Helper()
	return func(responseType discord.InteractionResponseType, data discord.InteractionResponseData, _ ...rest.RequestOpt) error {
		response.calls++
		response.responseType = responseType
		switch value := data.(type) {
		case discord.MessageCreate:
			response.message = value
		case discord.MessageUpdate:
			response.update = value
		default:
			t.Fatalf("unexpected birthday response data type %T", data)
		}
		return nil
	}
}

func requireEphemeralBirthdayResponse(t *testing.T, response *birthdayResponse) {
	t.Helper()
	require.Equal(t, 1, response.calls)
	require.Equal(t, discord.InteractionResponseTypeCreateMessage, response.responseType)
	require.True(t, response.message.Flags.Has(discord.MessageFlagEphemeral))
	require.NotNil(t, response.message.AllowedMentions)
	assert.Empty(t, response.message.AllowedMentions.Users)
}

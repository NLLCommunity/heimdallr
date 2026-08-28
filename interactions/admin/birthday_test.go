package admin

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
)

func TestAdminBirthdayCommandAndRoutes(t *testing.T) {
	command := Admin.Build().(discord.SlashCommandCreate)
	birthday := adminSubcommand(t, command, "birthday")
	assert.Equal(t, []string{"enabled", "channel", "time", "timezone", "reset"}, adminOptionNames(birthday.Options))
	assert.True(t, birthday.Options[2].(discord.ApplicationCommandOptionString).Autocomplete)
	assert.True(t, birthday.Options[3].(discord.ApplicationCommandOptionString).Autocomplete)

	birthdayMessage := adminSubcommand(t, command, "birthday-message")
	assert.Equal(t, []string{"reset"}, adminOptionNames(birthdayMessage.Options))

	router := handler.New()
	Interactions(router)
	assert.True(t, router.Match("/admin/birthday-message/button", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	assert.True(t, router.Match("/admin/birthday-message/modal", discord.InteractionTypeModalSubmit, 0))
}

func TestBirthdayTimeAutocompleteUsesQuarterHours(t *testing.T) {
	all, err := BirthdayTimeAutocomplete(rave.AutocompleteContext[string]{Value: ""})
	require.NoError(t, err)
	require.Len(t, all, 25)
	assert.Equal(t, "00:00", all[0].Value)

	ten, err := BirthdayTimeAutocomplete(rave.AutocompleteContext[string]{Value: "10:"})
	require.NoError(t, err)
	assert.Equal(t, []rave.Choice[string]{
		{Name: "10:00", Value: "10:00"},
		{Name: "10:15", Value: "10:15"},
		{Name: "10:30", Value: "10:30"},
		{Name: "10:45", Value: "10:45"},
	}, ten)
}

func TestBirthdayTimezoneAutocompleteUsesCanonicalCatalog(t *testing.T) {
	choices, err := BirthdayTimezoneAutocomplete(rave.AutocompleteContext[string]{Value: "oslo"})
	require.NoError(t, err)
	assert.Equal(t, []rave.Choice[string]{{Name: "Europe/Oslo", Value: "Europe/Oslo"}}, choices)
}

func TestAdminBirthdayHandlerValidatesBeforePartialWrite(t *testing.T) {
	setupAdminBirthdayDatabase(t)

	invalidTime, invalidResponse := newAdminBirthdayCommandEvent(t, "birthday", `
		{"name":"time","type":3,"value":"10:01"}
	`)
	require.NoError(t, AdminBirthdayHandler(invalidTime))
	requireAdminBirthdayEphemeral(t, invalidResponse)
	settings, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.Equal(t, model.DefaultBirthdayHour, settings.BirthdayHour)
	assert.Equal(t, model.DefaultBirthdayMinute, settings.BirthdayMinute)

	withoutChannel, enableResponse := newAdminBirthdayCommandEvent(t, "birthday", `
		{"name":"enabled","type":5,"value":true}
	`)
	require.NoError(t, AdminBirthdayHandler(withoutChannel))
	requireAdminBirthdayEphemeral(t, enableResponse)
	settings, err = model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.False(t, settings.BirthdayEnabled)

	valid, validResponse := newAdminBirthdayCommandEvent(t, "birthday", `
		{"name":"time","type":3,"value":"10:15"},
		{"name":"timezone","type":3,"value":"Europe/Oslo"}
	`)
	require.NoError(t, AdminBirthdayHandler(valid))
	requireAdminBirthdayEphemeral(t, validResponse)
	settings, err = model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.Equal(t, 10, settings.BirthdayHour)
	assert.Equal(t, 15, settings.BirthdayMinute)
	assert.Equal(t, "Europe/Oslo", settings.BirthdayTimezone)

	reset, resetResponse := newAdminBirthdayCommandEvent(t, "birthday", `
		{"name":"reset","type":3,"value":"all"}
	`)
	require.NoError(t, AdminBirthdayHandler(reset))
	requireAdminBirthdayEphemeral(t, resetResponse)
	settings, err = model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.Equal(t, model.DefaultBirthdayTimezone, settings.BirthdayTimezone)
	assert.Equal(t, model.DefaultBirthdayMinute, settings.BirthdayMinute)
}

func TestAdminBirthdayHandlerTrimsTimezone(t *testing.T) {
	setupAdminBirthdayDatabase(t)
	event, response := newAdminBirthdayCommandEvent(t, "birthday", `
		{"name":"timezone","type":3,"value":" Europe/Oslo "}
	`)

	require.NoError(t, AdminBirthdayHandler(event))
	requireAdminBirthdayEphemeral(t, response)
	assert.Equal(t, "Birthday timezone set to Europe/Oslo.", response.message.Content)

	settings, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.Equal(t, "Europe/Oslo", settings.BirthdayTimezone)
}

func TestAdminBirthdayNoOptionsShowsCurrentValues(t *testing.T) {
	setupAdminBirthdayDatabase(t)
	event, response := newAdminBirthdayCommandEvent(t, "birthday", "")
	require.NoError(t, AdminBirthdayHandler(event))
	requireAdminBirthdayEphemeral(t, response)
	assert.Contains(t, response.message.Content, "10:00")
	assert.Contains(t, response.message.Content, "UTC")
	assert.Contains(t, response.message.Content, "not set")
}

func TestAdminBirthdayMessageResetLeavesV2Mode(t *testing.T) {
	setupAdminBirthdayDatabase(t)
	settings, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	settings.BirthdayMessageV2 = true
	settings.BirthdayMessageV2Json = `[{"type":10,"content":"custom"}]`
	require.NoError(t, model.SetGuildSettings(settings))

	view, viewResponse := newAdminBirthdayCommandEvent(t, "birthday-message", "")
	require.NoError(t, AdminBirthdayMessageHandler(view))
	requireAdminBirthdayEphemeral(t, viewResponse)
	assert.Contains(t, viewResponse.message.Content, "dashboard")

	reset, resetResponse := newAdminBirthdayCommandEvent(t, "birthday-message", `
		{"name":"reset","type":3,"value":"reset"}
	`)
	require.NoError(t, AdminBirthdayMessageHandler(reset))
	requireAdminBirthdayEphemeral(t, resetResponse)
	settings, err = model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.False(t, settings.BirthdayMessageV2)
	assert.Empty(t, settings.BirthdayMessageV2Json)
	assert.Equal(t, model.DefaultBirthdayMessage, settings.BirthdayMessage)
}

func TestAdminBirthdayMessageModalRejectsEmptyMessage(t *testing.T) {
	setupAdminBirthdayDatabase(t)
	event, response := newAdminBirthdayModalEvent(t, "")

	require.NoError(t, AdminBirthdayMessageModalHandler(event))
	requireAdminBirthdayEphemeral(t, response)
	assert.Contains(t, response.message.Content, "cannot be empty")

	settings, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.Equal(t, model.DefaultBirthdayMessage, settings.BirthdayMessage)
}

func TestBirthdayInfoContainsConfigurationOnly(t *testing.T) {
	settings := &model.GuildSettings{}
	model.ResetBirthdaySettings(settings)
	info := birthdayInfo(settings)
	assert.Contains(t, info, "10:00")
	assert.Contains(t, info, "UTC")
	assert.NotContains(t, strings.ToLower(info), "user birthday")
}

func adminSubcommand(t *testing.T, command discord.SlashCommandCreate, name string) discord.ApplicationCommandOptionSubCommand {
	t.Helper()
	for _, option := range command.Options {
		if subcommand, ok := option.(discord.ApplicationCommandOptionSubCommand); ok && subcommand.Name == name {
			return subcommand
		}
	}
	t.Fatalf("admin subcommand %q not found", name)
	return discord.ApplicationCommandOptionSubCommand{}
}

func adminOptionNames(options []discord.ApplicationCommandOption) []string {
	names := make([]string, 0, len(options))
	for _, option := range options {
		names = append(names, option.OptionName())
	}
	return names
}

type adminBirthdayResponse struct {
	calls   int
	message discord.MessageCreate
}

func setupAdminBirthdayDatabase(t *testing.T) {
	t.Helper()
	original := model.DB
	db, err := model.InitDB(filepath.Join(t.TempDir(), "admin-birthdays.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = original
	})
}

func newAdminBirthdayCommandEvent(t *testing.T, subcommand, options string) (*handler.CommandEvent, *adminBirthdayResponse) {
	t.Helper()
	if strings.TrimSpace(options) != "" {
		options = ",\"options\":[" + options + "]"
	}
	interaction, err := discord.UnmarshalInteraction([]byte(fmt.Sprintf(`{
		"id":"1","application_id":"2","type":2,"token":"token","version":1,
		"guild_id":"10","channel":{"id":"30","type":0,"name":"general"},
		"member":{"user":{"id":"20","username":"admin","discriminator":"0"},"roles":[],"permissions":"8"},
		"data":{"id":"3","name":"admin","type":1,"options":[{"name":%q,"type":1%s}]}
	}`, subcommand, options)))
	require.NoError(t, err)

	caches := cache.New(cache.WithCaches(cache.FlagGuilds))
	caches.AddGuild(discord.Guild{ID: 10, Name: "Test Guild"})
	response := &adminBirthdayResponse{}
	client := &bot.Client{Caches: caches}
	event := &handler.CommandEvent{ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
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
	}}
	return event, response
}

func newAdminBirthdayModalEvent(t *testing.T, message string) (*handler.ModalEvent, *adminBirthdayResponse) {
	t.Helper()
	interaction, err := discord.UnmarshalInteraction([]byte(fmt.Sprintf(`{
		"id":"1","application_id":"2","type":5,"token":"token","version":1,
		"guild_id":"10","channel":{"id":"30","type":0,"name":"general"},
		"member":{"user":{"id":"20","username":"admin","discriminator":"0"},"roles":[],"permissions":"8"},
		"data":{"custom_id":"/admin/birthday-message/modal","components":[
			{"type":18,"label":"Birthday message","component":{"type":4,"custom_id":"message","style":2,"value":%q}}
		]}
	}`, message)))
	require.NoError(t, err)

	caches := cache.New(cache.WithCaches(cache.FlagGuilds))
	caches.AddGuild(discord.Guild{ID: 10, Name: "Test Guild"})
	response := &adminBirthdayResponse{}
	client := &bot.Client{Caches: caches}
	event := &handler.ModalEvent{ModalSubmitInteractionCreate: &events.ModalSubmitInteractionCreate{
		GenericEvent:           events.NewGenericEvent(client, 0, 0),
		ModalSubmitInteraction: interaction.(discord.ModalSubmitInteraction),
		Respond: func(responseType discord.InteractionResponseType, data discord.InteractionResponseData, _ ...rest.RequestOpt) error {
			require.Equal(t, discord.InteractionResponseTypeCreateMessage, responseType)
			created, ok := data.(discord.MessageCreate)
			require.True(t, ok)
			response.calls++
			response.message = created
			return nil
		},
	}}
	return event, response
}

func requireAdminBirthdayEphemeral(t *testing.T, response *adminBirthdayResponse) {
	t.Helper()
	require.Equal(t, 1, response.calls)
	assert.True(t, response.message.Flags.Has(discord.MessageFlagEphemeral))
	assert.NotNil(t, response.message.AllowedMentions)
}

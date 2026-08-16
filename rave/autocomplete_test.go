package rave_test

import (
	"errors"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

type capturedAutocompleteResponse struct {
	responseType discord.InteractionResponseType
	data         discord.InteractionResponseData
}

func newAutocompleteInteractionEvent(
	commandName string,
	options map[string]discord.AutocompleteOption,
	captured *capturedAutocompleteResponse,
) *handler.InteractionEvent {
	return &handler.InteractionEvent{
		InteractionCreate: &events.InteractionCreate{
			Interaction: discord.AutocompleteInteraction{
				Data: discord.AutocompleteInteractionData{
					CommandName: commandName,
					Options:     options,
				},
			},
			Respond: func(
				responseType discord.InteractionResponseType,
				data discord.InteractionResponseData,
				_ ...rest.RequestOpt,
			) error {
				captured.responseType = responseType
				captured.data = data
				return nil
			},
		},
		Vars: make(map[string]string),
	}
}

func muxHasAutocompleteRoute(mux handler.Router, path string) bool {
	return mux.Match(path, discord.InteractionTypeAutocomplete, 0)
}

func TestAutocompleteDispatchesFocusedStringOptionAndResponds(t *testing.T) {
	mux := handler.New()
	var received string
	command := rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("query", "Search query").
				Autocomplete(func(ctx rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
					received = ctx.Value
					return []rave.Choice[string]{{Name: "Hello", Value: "hello"}}, nil
				}),
		)

	built := command.Register(mux).(discord.SlashCommandCreate)

	require.True(t, muxHasAutocompleteRoute(mux, "/search"))
	stringOption := built.Options[0].(discord.ApplicationCommandOptionString)
	require.True(t, stringOption.Autocomplete)

	captured := &capturedAutocompleteResponse{}
	event := newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"query": {
			Name:    "query",
			Type:    discord.ApplicationCommandOptionTypeString,
			Value:   []byte(`"hel"`),
			Focused: true,
		},
	}, captured)

	err := mux.Handle("/search", event)

	require.NoError(t, err)
	require.Equal(t, "hel", received)
	require.Equal(t, discord.InteractionResponseTypeAutocompleteResult, captured.responseType)
	require.Equal(t, discord.AutocompleteResult{
		Choices: []discord.AutocompleteChoice{
			discord.AutocompleteChoiceString{Name: "Hello", Value: "hello"},
		},
	}, captured.data)
}

func TestAutocompleteDispatchesOnlyFocusedOption(t *testing.T) {
	mux := handler.New()
	queryCalled := false
	limitCalled := false
	command := rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("query", "Search query").
				Autocomplete(func(rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
					queryCalled = true
					return nil, nil
				}),
		).
		AddOptions(
			rave.OptionInt("limit", "Result limit").
				Autocomplete(func(ctx rave.AutocompleteContext[int]) ([]rave.Choice[int], error) {
					limitCalled = true
					require.Equal(t, 12, ctx.Value)
					return []rave.Choice[int]{{Name: "Twenty", Value: 20}}, nil
				}),
		)
	command.Register(mux)

	captured := &capturedAutocompleteResponse{}
	event := newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"query": {
			Name:  "query",
			Type:  discord.ApplicationCommandOptionTypeString,
			Value: []byte(`"ignored"`),
		},
		"limit": {
			Name:    "limit",
			Type:    discord.ApplicationCommandOptionTypeInt,
			Value:   []byte(`12`),
			Focused: true,
		},
	}, captured)

	err := mux.Handle("/search", event)

	require.NoError(t, err)
	require.False(t, queryCalled)
	require.True(t, limitCalled)
	require.Equal(t, discord.AutocompleteResult{
		Choices: []discord.AutocompleteChoice{
			discord.AutocompleteChoiceInt{Name: "Twenty", Value: 20},
		},
	}, captured.data)
}

func TestAutocompleteConvertsFloatChoices(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("scale", "Scale a value").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionFloat("factor", "Scale factor").
				Autocomplete(func(ctx rave.AutocompleteContext[float64]) ([]rave.Choice[float64], error) {
					require.Equal(t, 1.25, ctx.Value)
					return []rave.Choice[float64]{{Name: "Double", Value: 2.0}}, nil
				}),
		)
	command.Register(mux)

	captured := &capturedAutocompleteResponse{}
	event := newAutocompleteInteractionEvent("scale", map[string]discord.AutocompleteOption{
		"factor": {
			Name:    "factor",
			Type:    discord.ApplicationCommandOptionTypeFloat,
			Value:   []byte(`1.25`),
			Focused: true,
		},
	}, captured)

	err := mux.Handle("/scale", event)

	require.NoError(t, err)
	require.Equal(t, discord.AutocompleteResult{
		Choices: []discord.AutocompleteChoice{
			discord.AutocompleteChoiceFloat{Name: "Double", Value: 2.0},
		},
	}, captured.data)
}

func TestAutocompletePropagatesCallbackError(t *testing.T) {
	mux := handler.New()
	wantErr := errors.New("autocomplete failed")
	command := rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("query", "Search query").
				Autocomplete(func(rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
					return nil, wantErr
				}),
		)
	command.Register(mux)

	event := newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"query": {
			Name:    "query",
			Type:    discord.ApplicationCommandOptionTypeString,
			Value:   []byte(`"hel"`),
			Focused: true,
		},
	}, &capturedAutocompleteResponse{})

	err := mux.Handle("/search", event)

	require.ErrorIs(t, err, wantErr)
}

func TestAutocompleteRejectsMoreThanTwentyFiveChoices(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("query", "Search query").
				Autocomplete(func(rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
					return make([]rave.Choice[string], 26), nil
				}),
		)
	command.Register(mux)

	event := newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"query": {
			Name:    "query",
			Type:    discord.ApplicationCommandOptionTypeString,
			Value:   []byte(`"hel"`),
			Focused: true,
		},
	}, &capturedAutocompleteResponse{})

	err := mux.Handle("/search", event)

	require.ErrorIs(t, err, rave.ErrTooManyAutocompleteChoices)
}

func TestAutocompleteRouteIsNotRegisteredWithoutAutocompleteOptions(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(rave.OptionString("query", "Search query"))

	command.Register(mux)

	require.False(t, muxHasAutocompleteRoute(mux, "/search"))
}

func TestAutocompleteRegistersAtSubcommandPath(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("search", "Search settings").
				Handle(noopCommandHandler).
				AddOptions(
					rave.OptionString("query", "Search query").
						Autocomplete(func(rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
							return nil, nil
						}),
				),
		)

	command.Register(mux)

	require.False(t, muxHasAutocompleteRoute(mux, "/admin"))
	require.True(t, muxHasAutocompleteRoute(mux, "/admin/search"))
}

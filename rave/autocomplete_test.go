package rave_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
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
	calls        int
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
				captured.calls++
				captured.responseType = responseType
				captured.data = data
				return nil
			},
		},
		Vars: make(map[string]string),
	}
}

func dispatchStringAutocomplete(t *testing.T, choices []rave.Choice[string]) (*capturedAutocompleteResponse, error) {
	t.Helper()

	mux := handler.New()
	rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("query", "Search query").
				Autocomplete(func(rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
					return choices, nil
				}),
		).
		Register(mux)

	captured := &capturedAutocompleteResponse{}
	err := mux.Handle("/search", newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"query": {
			Name:    "query",
			Type:    discord.ApplicationCommandOptionTypeString,
			Value:   []byte(`"query"`),
			Focused: true,
		},
	}, captured))
	return captured, err
}

func dispatchIntAutocomplete(t *testing.T, choices []rave.Choice[int]) (*capturedAutocompleteResponse, error) {
	t.Helper()

	mux := handler.New()
	rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionInt("limit", "Result limit").
				Autocomplete(func(rave.AutocompleteContext[int]) ([]rave.Choice[int], error) {
					return choices, nil
				}),
		).
		Register(mux)

	captured := &capturedAutocompleteResponse{}
	err := mux.Handle("/search", newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"limit": {
			Name:    "limit",
			Type:    discord.ApplicationCommandOptionTypeInt,
			Value:   []byte(`1`),
			Focused: true,
		},
	}, captured))
	return captured, err
}

func dispatchFloatAutocomplete(t *testing.T, choices []rave.Choice[float64]) (*capturedAutocompleteResponse, error) {
	t.Helper()

	mux := handler.New()
	rave.Slash("search", "Search messages").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionFloat("factor", "Scale factor").
				Autocomplete(func(rave.AutocompleteContext[float64]) ([]rave.Choice[float64], error) {
					return choices, nil
				}),
		).
		Register(mux)

	captured := &capturedAutocompleteResponse{}
	err := mux.Handle("/search", newAutocompleteInteractionEvent("search", map[string]discord.AutocompleteOption{
		"factor": {
			Name:    "factor",
			Type:    discord.ApplicationCommandOptionTypeFloat,
			Value:   []byte(`1`),
			Focused: true,
		},
	}, captured))
	return captured, err
}

func requireInvalidAutocompleteChoice(
	t *testing.T,
	err error,
	captured *capturedAutocompleteResponse,
	index int,
	field string,
) {
	t.Helper()

	require.ErrorIs(t, err, rave.ErrInvalidAutocompleteChoice)
	require.ErrorContains(t, err, fmt.Sprintf("at index %d: invalid %s", index, field))
	require.Zero(t, captured.calls)
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

func TestAutocompleteRejectsInvalidChoiceNames(t *testing.T) {
	cases := []struct {
		name    string
		choices []rave.Choice[string]
		field   string
	}{
		{name: "empty", choices: []rave.Choice[string]{{Name: "First", Value: "first"}, {Name: "", Value: "value"}}, field: "name"},
		{name: "100 runes", choices: []rave.Choice[string]{{Name: strings.Repeat("界", 100), Value: "value"}}},
		{name: "101 runes", choices: []rave.Choice[string]{{Name: "First", Value: "first"}, {Name: strings.Repeat("界", 101), Value: "value"}}, field: "name"},
		{name: "invalid UTF-8", choices: []rave.Choice[string]{{Name: "First", Value: "first"}, {Name: string([]byte{0xff}), Value: "value"}}, field: "name"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured, err := dispatchStringAutocomplete(t, tc.choices)

			if tc.field != "" {
				requireInvalidAutocompleteChoice(t, err, captured, 1, tc.field)
				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, captured.calls)
		})
	}
}

func TestAutocompleteRejectsInvalidStringChoiceValues(t *testing.T) {
	cases := []struct {
		name    string
		choices []rave.Choice[string]
		field   string
	}{
		{name: "empty", choices: []rave.Choice[string]{{Name: "Choice", Value: ""}}},
		{name: "100 runes", choices: []rave.Choice[string]{{Name: "Choice", Value: strings.Repeat("界", 100)}}},
		{name: "101 runes", choices: []rave.Choice[string]{{Name: "First", Value: "first"}, {Name: "Choice", Value: strings.Repeat("界", 101)}}, field: "value"},
		{name: "invalid UTF-8", choices: []rave.Choice[string]{{Name: "First", Value: "first"}, {Name: "Choice", Value: string([]byte{0xff})}}, field: "value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured, err := dispatchStringAutocomplete(t, tc.choices)

			if tc.field != "" {
				requireInvalidAutocompleteChoice(t, err, captured, 1, tc.field)
				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, captured.calls)
		})
	}
}

func TestAutocompleteRejectsOutOfRangeIntegerChoiceValues(t *testing.T) {
	const maxSafeInteger = 1<<53 - 1

	cases := []struct {
		name    string
		choices []rave.Choice[int]
		field   string
	}{
		{name: "minimum", choices: []rave.Choice[int]{{Name: "Minimum", Value: -maxSafeInteger}}},
		{name: "maximum", choices: []rave.Choice[int]{{Name: "Maximum", Value: maxSafeInteger}}},
		{name: "below minimum", choices: []rave.Choice[int]{{Name: "First", Value: 1}, {Name: "Below", Value: -maxSafeInteger - 1}}, field: "value"},
		{name: "above maximum", choices: []rave.Choice[int]{{Name: "First", Value: 1}, {Name: "Above", Value: maxSafeInteger + 1}}, field: "value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured, err := dispatchIntAutocomplete(t, tc.choices)

			if tc.field != "" {
				requireInvalidAutocompleteChoice(t, err, captured, 1, tc.field)
				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, captured.calls)
		})
	}
}

func TestAutocompleteRejectsInvalidNumberChoiceValues(t *testing.T) {
	const maxNumber = 1 << 53

	cases := []struct {
		name    string
		choices []rave.Choice[float64]
		field   string
	}{
		{name: "minimum", choices: []rave.Choice[float64]{{Name: "Minimum", Value: -maxNumber}}},
		{name: "maximum", choices: []rave.Choice[float64]{{Name: "Maximum", Value: maxNumber}}},
		{name: "below minimum", choices: []rave.Choice[float64]{{Name: "First", Value: 1}, {Name: "Below", Value: math.Nextafter(-maxNumber, math.Inf(-1))}}, field: "value"},
		{name: "above maximum", choices: []rave.Choice[float64]{{Name: "First", Value: 1}, {Name: "Above", Value: math.Nextafter(maxNumber, math.Inf(1))}}, field: "value"},
		{name: "NaN", choices: []rave.Choice[float64]{{Name: "First", Value: 1}, {Name: "NaN", Value: math.NaN()}}, field: "value"},
		{name: "positive infinity", choices: []rave.Choice[float64]{{Name: "First", Value: 1}, {Name: "Positive infinity", Value: math.Inf(1)}}, field: "value"},
		{name: "negative infinity", choices: []rave.Choice[float64]{{Name: "First", Value: 1}, {Name: "Negative infinity", Value: math.Inf(-1)}}, field: "value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured, err := dispatchFloatAutocomplete(t, tc.choices)

			if tc.field != "" {
				requireInvalidAutocompleteChoice(t, err, captured, 1, tc.field)
				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, captured.calls)
		})
	}
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

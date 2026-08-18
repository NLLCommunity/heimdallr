package rave

import (
	"errors"
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

var (
	ErrAutocompleteHandlerNotFound = errors.New("autocomplete handler not found")
	ErrTooManyAutocompleteChoices  = errors.New("autocomplete cannot return more than 25 choices")
	ErrInvalidAutocompleteChoice   = errors.New("invalid autocomplete choice")
)

type AutocompleteValue interface {
	~string | ~int | ~float64
}

type AutocompleteContext[T AutocompleteValue] struct {
	Event *handler.AutocompleteEvent
	Value T
}

type Choice[T AutocompleteValue] struct {
	Name  string
	Value T
}

type AutocompleteHandler[T AutocompleteValue] func(AutocompleteContext[T]) ([]Choice[T], error)

type internalAutocompleteHandler func(
	e *handler.AutocompleteEvent,
	focused discord.AutocompleteOption,
) error

type autocompleteProvider interface {
	autocompleteBinding() (name string, h internalAutocompleteHandler, ok bool)
}

func adaptAutocomplete[T AutocompleteValue](
	h AutocompleteHandler[T],
	parse func(discord.AutocompleteOption) T,
	convert func(Choice[T]) discord.AutocompleteChoice,
) internalAutocompleteHandler {
	if h == nil {
		panic("autocomplete handler cannot be nil")
	}

	return func(e *handler.AutocompleteEvent, focused discord.AutocompleteOption) error {
		choices, err := h(AutocompleteContext[T]{
			Event: e,
			Value: parse(focused),
		})
		if err != nil {
			return err
		}
		if len(choices) > 25 {
			return fmt.Errorf("%w: got %d", ErrTooManyAutocompleteChoices, len(choices))
		}
		for i, choice := range choices {
			if !validDiscordChoiceName(choice.Name) {
				return fmt.Errorf("%w at index %d: invalid name", ErrInvalidAutocompleteChoice, i)
			}
			if !validAutocompleteChoiceValue(choice.Value) {
				return fmt.Errorf("%w at index %d: invalid value", ErrInvalidAutocompleteChoice, i)
			}
		}

		result := make([]discord.AutocompleteChoice, len(choices))
		for i, choice := range choices {
			result[i] = convert(choice)
		}
		return e.AutocompleteResult(result)
	}
}

func validAutocompleteChoiceValue[T AutocompleteValue](value T) bool {
	switch value := any(value).(type) {
	case string:
		return validDiscordStringChoiceValue(value)
	case int:
		return validDiscordIntegerChoiceValue(value)
	case float64:
		return validDiscordNumberChoiceValue(value)
	default:
		return false
	}
}

func parseStringAutocomplete(option discord.AutocompleteOption) string {
	return option.String()
}

func parseIntAutocomplete(option discord.AutocompleteOption) int {
	return option.Int()
}

func parseFloatAutocomplete(option discord.AutocompleteOption) float64 {
	return option.Float()
}

func convertStringChoice(choice Choice[string]) discord.AutocompleteChoice {
	return discord.AutocompleteChoiceString{Name: choice.Name, Value: choice.Value}
}

func convertIntChoice(choice Choice[int]) discord.AutocompleteChoice {
	return discord.AutocompleteChoiceInt{Name: choice.Name, Value: choice.Value}
}

func convertFloatChoice(choice Choice[float64]) discord.AutocompleteChoice {
	return discord.AutocompleteChoiceFloat{Name: choice.Name, Value: choice.Value}
}

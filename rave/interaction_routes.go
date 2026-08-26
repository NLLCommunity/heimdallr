package rave

import (
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

type customIDRoute[T any] struct {
	pattern string
}

type ComponentRouteBuilder[T any] struct {
	customIDRoute[T]
	handler       handler.ComponentHandler
	buttonHandler handler.ButtonComponentHandler
}

type ModalRouteBuilder[T any] struct {
	customIDRoute[T]
	handler handler.ModalHandler
}

func Component(pattern string) *ComponentRouteBuilder[Vars] {
	return ComponentOf[Vars](pattern)
}

func ComponentOf[T any](pattern string) *ComponentRouteBuilder[T] {
	return &ComponentRouteBuilder[T]{customIDRoute: customIDRoute[T]{pattern: pattern}}
}

func Modal(pattern string) *ModalRouteBuilder[Vars] {
	return ModalOf[Vars](pattern)
}

func ModalOf[T any](pattern string) *ModalRouteBuilder[T] {
	return &ModalRouteBuilder[T]{customIDRoute: customIDRoute[T]{pattern: pattern}}
}

func (r customIDRoute[T]) CustomID(values T) (string, error) {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		return "", err
	}

	if err := validateCustomIDSchema[T](pattern); err != nil {
		return "", err
	}
	if reflect.TypeFor[T]() == reflect.TypeFor[Vars]() {
		vars := any(values).(Vars)
		return customIDFromVars(pattern, vars)
	}
	vars, err := varsFromStruct(values)
	if err != nil {
		return "", err
	}
	return customIDFromVars(pattern, vars)
}

func (r customIDRoute[T]) CustomIDVars(values Vars) (string, error) {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		return "", err
	}
	return customIDFromVars(pattern, values)
}

func (r customIDRoute[T]) StaticCustomID() string {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		panic(err)
	}
	if len(pattern.placeholders) != 0 {
		panic(fmt.Errorf("%w: static custom ID cannot contain placeholders", ErrInvalidCustomIDPattern))
	}
	if err := validateCustomIDPatternLength[T](pattern); err != nil {
		panic(err)
	}
	return r.pattern
}

func validateCustomIDPatternLength[T any](pattern customIDPattern) error {
	placeholderMinimums := typedPlaceholderMinimumRunes[T]()
	minimumRunes := 1 // leading slash
	for index, segment := range pattern.segments {
		if index > 0 {
			minimumRunes++ // segment separator
		}
		if name, placeholder := pattern.placeholders[index]; placeholder {
			minimum := placeholderMinimums[name]
			if minimum == 0 {
				minimum = 1
			}
			minimumRunes += minimum
		} else {
			minimumRunes += utf8.RuneCountInString(segment)
		}
	}
	if minimumRunes > maxCustomIDRunes {
		return ErrCustomIDTooLong
	}
	return nil
}

func typedPlaceholderMinimumRunes[T any]() map[string]int {
	typeOfValue := reflect.TypeFor[T]()
	if typeOfValue == reflect.TypeFor[Vars]() {
		return nil
	}
	for typeOfValue.Kind() == reflect.Pointer {
		typeOfValue = typeOfValue.Elem()
	}
	if typeOfValue.Kind() != reflect.Struct {
		return nil
	}

	minimums := make(map[string]int)
	for index := range typeOfValue.NumField() {
		field := typeOfValue.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, ok := optionName(field)
		if !ok {
			continue
		}
		minimums[name] = encodedTypeMinimumRunes(field.Type)
	}
	return minimums
}

func encodedTypeMinimumRunes(typeOfValue reflect.Type) int {
	// encodeCustomIDValue checks these interfaces on the original value before
	// dereferencing pointers, so the schema check must use the same dispatch order.
	if !isCustomIDTypeEncodable(typeOfValue) || customIDTypeUsesCustomEncoder(typeOfValue) {
		return 1
	}
	for typeOfValue.Kind() == reflect.Pointer {
		typeOfValue = typeOfValue.Elem()
	}
	if typeOfValue.Kind() == reflect.Bool {
		return len("true")
	}
	return 1
}

func (r *ComponentRouteBuilder[T]) Handle(h handler.ComponentHandler) *ComponentRouteBuilder[T] {
	rejectMixedHandlerStyles(h != nil, r.buttonHandler != nil, "component")
	r.handler = h
	return r
}

func (r *ComponentRouteBuilder[T]) HandleButton(h handler.ButtonComponentHandler) *ComponentRouteBuilder[T] {
	rejectMixedHandlerStyles(h != nil, r.handler != nil, "component")
	r.buttonHandler = h
	return r
}

func (r *ModalRouteBuilder[T]) Handle(h handler.ModalHandler) *ModalRouteBuilder[T] {
	r.handler = h
	return r
}

func (r *ComponentRouteBuilder[T]) register(router handler.Router) (command discord.ApplicationCommandCreate, hasCommand bool) {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		panic(err)
	}
	if err := validateCustomIDSchema[T](pattern); err != nil {
		panic(err)
	}
	if err := validateCustomIDPatternLength[T](pattern); err != nil {
		panic(err)
	}
	if r.buttonHandler != nil {
		router.Route(r.pattern, func(exact handler.Router) {
			exact.ButtonComponent("/", func(e discord.ButtonInteractionData, event *handler.ComponentEvent) error {
				if !pattern.matchesExactly(event.Data.CustomID()) {
					return nil
				}
				return r.buttonHandler(e, event)
			})
		})
		return
	}
	if r.handler != nil {
		router.Route(r.pattern, func(exact handler.Router) {
			exact.Component("/", func(event *handler.ComponentEvent) error {
				if !pattern.matchesExactly(event.Data.CustomID()) {
					return nil
				}
				return r.handler(event)
			})
		})
		return
	}
	panic("component route is missing a handler: " + r.pattern)
}

func (r *ComponentRouteBuilder[T]) Register(router handler.Router) {
	r.register(router)
}

func (r *ModalRouteBuilder[T]) register(router handler.Router) (command discord.ApplicationCommandCreate, hasCommand bool) {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		panic(err)
	}
	if err := validateCustomIDSchema[T](pattern); err != nil {
		panic(err)
	}
	if err := validateCustomIDPatternLength[T](pattern); err != nil {
		panic(err)
	}
	if r.handler == nil {
		panic("modal route is missing a handler: " + r.pattern)
	}
	router.Route(r.pattern, func(exact handler.Router) {
		exact.Modal("/", func(event *handler.ModalEvent) error {
			if !pattern.matchesExactly(event.Data.CustomID) {
				return nil
			}
			return r.handler(event)
		})
	})
	return nil, false
}

func (r *ModalRouteBuilder[T]) Register(router handler.Router) {
	r.register(router)
}

package rave

import (
	"fmt"
	"reflect"
	"unicode/utf8"

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
	if err := validateLiteralCustomIDLength(pattern); err != nil {
		panic(err)
	}
	return r.pattern
}

func validateLiteralCustomIDLength(pattern customIDPattern) error {
	if len(pattern.placeholders) == 0 && utf8.RuneCountInString(pattern.raw) > maxCustomIDRunes {
		return ErrCustomIDTooLong
	}
	return nil
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

func (r *ComponentRouteBuilder[T]) Register(router handler.Router) {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		panic(err)
	}
	if err := validateCustomIDSchema[T](pattern); err != nil {
		panic(err)
	}
	if err := validateLiteralCustomIDLength(pattern); err != nil {
		panic(err)
	}
	if r.buttonHandler != nil {
		router.ButtonComponent(r.pattern, r.buttonHandler)
		return
	}
	if r.handler != nil {
		router.Component(r.pattern, r.handler)
		return
	}
	panic("component route is missing a handler: " + r.pattern)
}

func (r *ModalRouteBuilder[T]) Register(router handler.Router) {
	pattern, err := compileCustomIDPattern(r.pattern)
	if err != nil {
		panic(err)
	}
	if err := validateCustomIDSchema[T](pattern); err != nil {
		panic(err)
	}
	if err := validateLiteralCustomIDLength(pattern); err != nil {
		panic(err)
	}
	if r.handler == nil {
		panic("modal route is missing a handler: " + r.pattern)
	}
	router.Modal(r.pattern, r.handler)
}

package rave_test

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

func noopComponentHandler(*handler.ComponentEvent) error { return nil }

func noopButtonHandler(discord.ButtonInteractionData, *handler.ComponentEvent) error {
	return nil
}

func noopModalHandler(*handler.ModalEvent) error { return nil }

func TestComponentRouteRegistersGenericHandler(t *testing.T) {
	router := handler.New()
	rave.Component("/button/{id}").Handle(noopComponentHandler).Register(router)

	require.True(t, router.Match("/button/1", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	require.True(t, router.Match("/button/1", discord.InteractionTypeComponent, int(discord.ComponentTypeStringSelectMenu)))
}

func TestComponentRouteRegistersButtonHandler(t *testing.T) {
	router := handler.New()
	rave.Component("/button/{id}").HandleButton(noopButtonHandler).Register(router)

	require.True(t, router.Match("/button/1", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	require.False(t, router.Match("/button/1", discord.InteractionTypeComponent, int(discord.ComponentTypeStringSelectMenu)))
}

func TestModalRouteRegistersHandler(t *testing.T) {
	router := handler.New()
	rave.Modal("/modal/{id}").Handle(noopModalHandler).Register(router)

	require.True(t, router.Match("/modal/1", discord.InteractionTypeModalSubmit, 0))
}

type typedRouteVars struct {
	ChannelID uint64 `rave:"channelID"`
	MessageID uint64 `rave:"messageID"`
}

func TestRouteCustomIDVariants(t *testing.T) {
	untyped := rave.Component("/component/{id}")
	id, err := untyped.CustomID(rave.Vars{"id": 7})
	require.NoError(t, err)
	require.Equal(t, "/component/7", id)

	typed := rave.ModalOf[typedRouteVars]("/modal/{channelID}/{messageID}")
	id, err = typed.CustomID(typedRouteVars{ChannelID: 8, MessageID: 9})
	require.NoError(t, err)
	require.Equal(t, "/modal/8/9", id)

	id, err = typed.CustomIDVars(rave.Vars{"channelID": 10, "messageID": 11})
	require.NoError(t, err)
	require.Equal(t, "/modal/10/11", id)
}

func TestTypedRouteCustomIDRejectsRuntimeVarsForAnySchema(t *testing.T) {
	route := rave.ComponentOf[any]("/component/{id}")

	_, err := route.CustomID(rave.Vars{"id": 7})
	require.ErrorIs(t, err, rave.ErrInvalidCustomIDValues)
}

func TestRouteRegistrationRejectsVarsPointerSchema(t *testing.T) {
	route := rave.ComponentOf[*rave.Vars]("/component/{id}").Handle(noopComponentHandler)

	require.Panics(t, func() {
		route.Register(handler.New())
	})
}

func TestStaticCustomID(t *testing.T) {
	require.Equal(t, "/admin/show", rave.Component("/admin/show").StaticCustomID())
	require.Panics(t, func() {
		rave.Component("/admin/{action}").StaticCustomID()
	})
	require.Panics(t, func() {
		rave.Component("/" + strings.Repeat("å", 100)).StaticCustomID()
	})
}

func TestComponentRouteRegistrationRejectsOverlongStaticCustomID(t *testing.T) {
	pattern := "/" + strings.Repeat("å", 100)

	require.Panics(t, func() {
		rave.Component(pattern).Handle(noopComponentHandler).Register(handler.New())
	})
}

func TestModalRouteRegistrationRejectsOverlongStaticCustomID(t *testing.T) {
	pattern := "/" + strings.Repeat("å", 100)

	require.Panics(t, func() {
		rave.Modal(pattern).Handle(noopModalHandler).Register(handler.New())
	})
}

func TestRouteRegistrationAllowsOverlongDynamicPatterns(t *testing.T) {
	pattern := "/" + strings.Repeat("å", 100) + "/{id}"

	require.NotPanics(t, func() {
		rave.Component(pattern).Handle(noopComponentHandler).Register(handler.New())
	})
	require.NotPanics(t, func() {
		rave.Modal(pattern).Handle(noopModalHandler).Register(handler.New())
	})
}

func TestRouteRegistrationRejectsMissingAndMixedHandlers(t *testing.T) {
	require.Panics(t, func() {
		rave.Component("/button").Register(handler.New())
	})
	require.Panics(t, func() {
		rave.Modal("/modal").Register(handler.New())
	})
	require.Panics(t, func() {
		rave.Component("/button").
			Handle(noopComponentHandler).
			HandleButton(noopButtonHandler)
	})
	require.Panics(t, func() {
		rave.Component("invalid").Handle(noopComponentHandler).Register(handler.New())
	})
	type mismatched struct {
		Other string
	}
	require.Panics(t, func() {
		rave.ModalOf[mismatched]("/modal/{id}").Handle(noopModalHandler).Register(handler.New())
	})
}

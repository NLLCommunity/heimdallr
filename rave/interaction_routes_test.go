package rave_test

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/assert"
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

type typedBoolRouteVars struct {
	Enabled bool `rave:"enabled"`
}

type textEncodedBool bool

func (textEncodedBool) MarshalText() ([]byte, error) { return []byte("t"), nil }

type textEncodedBoolRouteVars struct {
	Enabled textEncodedBool `rave:"enabled"`
}

type pointerStringEncodedBool bool

func (*pointerStringEncodedBool) String() string { return "s" }

type pointerStringEncodedBoolRouteVars struct {
	Enabled *pointerStringEncodedBool `rave:"enabled"`
}

type scalarPointerStringerBool bool

func (*scalarPointerStringerBool) String() string { return "s" }

type scalarPointerStringerBoolRouteVars struct {
	Enabled scalarPointerStringerBool `rave:"enabled"`
}

type nestedTextEncodedBoolRouteVars struct {
	Enabled **textEncodedBool `rave:"enabled"`
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

func TestRouteRegistrationAllowsDynamicPatternAtMinimumCapacityLimit(t *testing.T) {
	// 1 leading slash + 95 fixed runes + 2 separators + 2 one-rune values = 100.
	pattern := "/" + strings.Repeat("å", 95) + "/{first}/{second}"

	require.NotPanics(t, func() {
		rave.Component(pattern).Handle(noopComponentHandler).Register(handler.New())
	})
	require.NotPanics(t, func() {
		rave.Modal(pattern).Handle(noopModalHandler).Register(handler.New())
	})
}

func TestRouteRegistrationRejectsDynamicPatternWithoutMinimumValueCapacity(t *testing.T) {
	// 1 leading slash + 96 fixed runes + 2 separators + 2 one-rune values = 101.
	pattern := "/" + strings.Repeat("å", 96) + "/{first}/{second}"

	assert.Panics(t, func() {
		rave.Component(pattern).Handle(noopComponentHandler).Register(handler.New())
	})
	assert.Panics(t, func() {
		rave.Modal(pattern).Handle(noopModalHandler).Register(handler.New())
	})
}

func TestTypedRouteRegistrationRejectsPatternWithoutPlainBoolCapacity(t *testing.T) {
	// A one-rune value would total 100, but the shortest plain bool is "true" (4), totaling 103.
	pattern := "/" + strings.Repeat("å", 97) + "/{enabled}"

	assert.Panics(t, func() {
		rave.ComponentOf[typedBoolRouteVars](pattern).
			Handle(noopComponentHandler).
			Register(handler.New())
	})
	assert.Panics(t, func() {
		rave.ModalOf[typedBoolRouteVars](pattern).
			Handle(noopModalHandler).
			Register(handler.New())
	})
}

func TestTypedRouteRegistrationUsesCustomBoolEncodingCapacity(t *testing.T) {
	// Both custom encoders emit one rune, so the rendered IDs fit exactly at 100.
	pattern := "/" + strings.Repeat("å", 97) + "/{enabled}"

	component := rave.ComponentOf[textEncodedBoolRouteVars](pattern).Handle(noopComponentHandler)
	require.NotPanics(t, func() {
		component.Register(handler.New())
	})
	id, err := component.CustomID(textEncodedBoolRouteVars{Enabled: true})
	require.NoError(t, err)
	require.Equal(t, "/"+strings.Repeat("å", 97)+"/t", id)

	encoded := pointerStringEncodedBool(true)
	modal := rave.ModalOf[pointerStringEncodedBoolRouteVars](pattern).Handle(noopModalHandler)
	require.NotPanics(t, func() {
		modal.Register(handler.New())
	})
	id, err = modal.CustomID(pointerStringEncodedBoolRouteVars{Enabled: &encoded})
	require.NoError(t, err)
	require.Equal(t, "/"+strings.Repeat("å", 97)+"/s", id)
}

func TestTypedRouteRegistrationMatchesEncoderPointerDispatchForBoolAliases(t *testing.T) {
	// encodeCustomIDValue checks interfaces before dereferencing. Neither a scalar with
	// pointer-only methods nor **T implements the inner type's custom encoder.
	pattern := "/" + strings.Repeat("å", 97) + "/{enabled}"

	assert.Panics(t, func() {
		rave.ComponentOf[scalarPointerStringerBoolRouteVars](pattern).
			Handle(noopComponentHandler).
			Register(handler.New())
	})
	assert.Panics(t, func() {
		rave.ModalOf[nestedTextEncodedBoolRouteVars](pattern).
			Handle(noopModalHandler).
			Register(handler.New())
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

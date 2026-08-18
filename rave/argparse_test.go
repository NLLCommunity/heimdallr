package rave_test

import (
	"encoding/json"
	"testing"

	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/require"
)

var UserNoteCommand = discord.SlashCommandCreate{
	Name:        "note-add",
	Description: "Add a note",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "target-user",
			Description: "The user to make a note about",
			Required:    true,
		},
		discord.ApplicationCommandOptionString{
			Name:        "note",
			Description: "The note to add",
			Required:    true,
		},
	},
}

type UserNoteArgs struct {
	TargetUser discord.Member `rave:"target-user"`
	Note       string
	Timespamp  uint `rave:"-"`
}

var e *handler.CommandEvent

type numericOptionFixture struct {
	name       string
	optionType discord.ApplicationCommandOptionType
	value      string
}

func commandEventWithNumericOptions(t *testing.T, options ...numericOptionFixture) *handler.CommandEvent {
	t.Helper()

	type rawOption struct {
		Name  string                               `json:"name"`
		Type  discord.ApplicationCommandOptionType `json:"type"`
		Value json.RawMessage                      `json:"value"`
	}
	rawOptions := make([]rawOption, len(options))
	for i, option := range options {
		rawOptions[i] = rawOption{
			Name:  option.name,
			Type:  option.optionType,
			Value: json.RawMessage(option.value),
		}
	}
	payload, err := json.Marshal(struct {
		ID      string                         `json:"id"`
		Name    string                         `json:"name"`
		Type    discord.ApplicationCommandType `json:"type"`
		Options []rawOption                    `json:"options"`
	}{
		ID:      "1",
		Name:    "numeric-test",
		Type:    discord.ApplicationCommandTypeSlash,
		Options: rawOptions,
	})
	require.NoError(t, err)

	var data discord.SlashCommandInteractionData
	require.NoError(t, json.Unmarshal(payload, &data))

	return &handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			ApplicationCommandInteraction: discord.ApplicationCommandInteraction{
				Data: data,
			},
		},
	}
}

func TestParseSlashCommandArgsParsesSupportedSignedForms(t *testing.T) {
	type args struct {
		Scalar  int8 `rave:"scalar"`
		Pointer *int `rave:"pointer"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "scalar", optionType: discord.ApplicationCommandOptionTypeInt, value: "-8"},
			numericOptionFixture{name: "pointer", optionType: discord.ApplicationCommandOptionTypeInt, value: "7"},
		),
	)

	require.NoError(t, err)
	require.Equal(t, int8(-8), data.Scalar)
	require.NotNil(t, data.Pointer)
	require.Equal(t, 7, *data.Pointer)
}

func TestParseSlashCommandArgsParsesSupportedUnsignedForms(t *testing.T) {
	type args struct {
		Scalar  uint8   `rave:"scalar"`
		Pointer *uint64 `rave:"pointer"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "scalar", optionType: discord.ApplicationCommandOptionTypeInt, value: "8"},
			numericOptionFixture{name: "pointer", optionType: discord.ApplicationCommandOptionTypeInt, value: "9"},
		),
	)

	require.NoError(t, err)
	require.Equal(t, uint8(8), data.Scalar)
	require.NotNil(t, data.Pointer)
	require.Equal(t, uint64(9), *data.Pointer)
}

func TestParseSlashCommandArgsParsesSupportedFloatForms(t *testing.T) {
	type args struct {
		Ordinary float32  `rave:"ordinary"`
		Boundary *float32 `rave:"boundary"`
		Large    float64  `rave:"large"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "ordinary", optionType: discord.ApplicationCommandOptionTypeFloat, value: "1.5"},
			numericOptionFixture{name: "boundary", optionType: discord.ApplicationCommandOptionTypeFloat, value: "3.4028234663852886e+38"},
			numericOptionFixture{name: "large", optionType: discord.ApplicationCommandOptionTypeFloat, value: "1.7976931348623157e+308"},
		),
	)

	require.NoError(t, err)
	require.Equal(t, float32(1.5), data.Ordinary)
	require.NotNil(t, data.Boundary)
	require.Equal(t, float32(3.4028234663852886e+38), *data.Boundary)
	require.Equal(t, 1.7976931348623157e+308, data.Large)
}

func TestParseSlashCommandArgsRejectsNestedSignedPointersWithoutPanicking(t *testing.T) {
	type args struct {
		Count **int `rave:"count"`
	}

	var (
		data *args
		err  error
	)
	require.NotPanics(t, func() {
		data, err = rave.ParseSlashCommandArgs[args](
			commandEventWithNumericOptions(t,
				numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "7"},
			),
		)
	})

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Count)
}

func TestParseSlashCommandArgsRejectsNestedUnsignedPointersWithoutPanicking(t *testing.T) {
	type args struct {
		Count **uint `rave:"count"`
	}

	var (
		data *args
		err  error
	)
	require.NotPanics(t, func() {
		data, err = rave.ParseSlashCommandArgs[args](
			commandEventWithNumericOptions(t,
				numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "7"},
			),
		)
	})

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Count)
}

func TestParseSlashCommandArgsRejectsNestedFloatPointersWithoutPanicking(t *testing.T) {
	type args struct {
		Ratio **float64 `rave:"ratio"`
	}

	var (
		data *args
		err  error
	)
	require.NotPanics(t, func() {
		data, err = rave.ParseSlashCommandArgs[args](
			commandEventWithNumericOptions(t,
				numericOptionFixture{name: "ratio", optionType: discord.ApplicationCommandOptionTypeFloat, value: "1.5"},
			),
		)
	})

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Ratio)
}

func TestParseSlashCommandArgsRejectsNegativeUnsignedWithoutMutation(t *testing.T) {
	type args struct {
		Count *uint64 `rave:"count"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "-1"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Count)
}

func TestParseSlashCommandArgsRejectsSignedOverflowWithoutMutation(t *testing.T) {
	type args struct {
		Count *int8 `rave:"count"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "128"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Count)
}

func TestParseSlashCommandArgsRejectsUnsignedOverflowWithoutMutation(t *testing.T) {
	type args struct {
		Count *uint8 `rave:"count"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "256"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Count)
}

func TestParseSlashCommandArgsRejectsFloatOverflowWithoutMutation(t *testing.T) {
	type args struct {
		Ratio *float32 `rave:"ratio"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "ratio", optionType: discord.ApplicationCommandOptionTypeFloat, value: "1.7976931348623157e+308"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Nil(t, data.Ratio)
}

func TestParseSlashCommandArgsRejectsNegativeUnsignedScalar(t *testing.T) {
	type args struct {
		Count uint64 `rave:"count"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "-1"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Equal(t, uint64(0), data.Count)
}

func TestParseSlashCommandArgsRejectsSignedScalarOverflow(t *testing.T) {
	type args struct {
		Count int8 `rave:"count"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "128"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Equal(t, int8(0), data.Count)
}

func TestParseSlashCommandArgsRejectsUnsignedScalarOverflow(t *testing.T) {
	type args struct {
		Count uint8 `rave:"count"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "count", optionType: discord.ApplicationCommandOptionTypeInt, value: "256"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Equal(t, uint8(0), data.Count)
}

func TestParseSlashCommandArgsRejectsFloatScalarOverflow(t *testing.T) {
	type args struct {
		Ratio float32 `rave:"ratio"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "ratio", optionType: discord.ApplicationCommandOptionTypeFloat, value: "1.7976931348623157e+308"},
		),
	)

	require.ErrorIs(t, err, rave.ErrNumericConversion)
	require.NotNil(t, data)
	require.Equal(t, float32(0), data.Ratio)
}

func TestParseSlashCommandArgsAssignsMentionableValuesByDeclaredType(t *testing.T) {
	type args struct {
		User   discord.MentionableValue  `rave:"user"`
		Role   *discord.MentionableValue `rave:"role"`
		Member *discord.MentionableValue `rave:"member"`
	}

	var interactionData discord.SlashCommandInteractionData
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "1",
		"name": "mentionable-test",
		"type": 1,
		"resolved": {
			"users": {
				"101": {"id": "101", "username": "user"},
				"103": {"id": "103", "username": "member"}
			},
			"roles": {
				"102": {"id": "102", "name": "role"}
			},
			"members": {
				"103": {"roles": []}
			}
		},
		"options": [
			{"name": "user", "type": 9, "value": "101"},
			{"name": "role", "type": 9, "value": "102"},
			{"name": "member", "type": 9, "value": "103"}
		]
	}`), &interactionData))

	data, err := rave.ParseSlashCommandArgs[args](&handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			ApplicationCommandInteraction: discord.ApplicationCommandInteraction{Data: interactionData},
		},
	})

	require.NoError(t, err)
	require.IsType(t, discord.User{}, data.User)
	require.NotNil(t, data.Role)
	require.IsType(t, discord.Role{}, *data.Role)
	require.NotNil(t, data.Member)
	require.IsType(t, discord.ResolvedMember{}, *data.Member)
}

func TestParseSlashCommandArgsAssignsNamedPrimitivePointers(t *testing.T) {
	type name string
	type enabled bool
	type args struct {
		Name    *name    `rave:"name"`
		Enabled *enabled `rave:"enabled"`
	}

	data, err := rave.ParseSlashCommandArgs[args](
		commandEventWithNumericOptions(t,
			numericOptionFixture{name: "name", optionType: discord.ApplicationCommandOptionTypeString, value: `"rune"`},
			numericOptionFixture{name: "enabled", optionType: discord.ApplicationCommandOptionTypeBool, value: "true"},
		),
	)

	require.NoError(t, err)
	require.NotNil(t, data.Name)
	require.Equal(t, name("rune"), *data.Name)
	require.NotNil(t, data.Enabled)
	require.Equal(t, enabled(true), *data.Enabled)
}

func TestParseSlashCommandArgsRejectsUnsupportedFieldType(t *testing.T) {
	type args struct {
		Channel discord.Channel `rave:"channel"`
	}

	event := &handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			ApplicationCommandInteraction: discord.ApplicationCommandInteraction{
				Data: discord.SlashCommandInteractionData{},
			},
		},
	}

	data, err := rave.ParseSlashCommandArgs[args](event)

	require.Nil(t, data)
	require.ErrorIs(t, err, rave.ErrUnsupportedFieldType)
	require.EqualError(t, err, `unsupported slash command argument field type: field Channel (option "channel") has type discord.Channel`)
}

func ExampleParseSlashCommandArgs() {
	args, err := rave.ParseSlashCommandArgs[UserNoteArgs](e)
	_, _ = args, err
}

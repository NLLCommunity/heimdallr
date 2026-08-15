package rave_test

import (
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

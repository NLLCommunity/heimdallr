package rave_test

import (
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
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

func ExampleParseSlashCommandArgs() {
	args, err := rave.ParseSlashCommandArgs[UserNoteArgs](e)
	_, _ = args, err
}

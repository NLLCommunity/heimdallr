package rave_test

import (
	"reflect"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

func noopCommandHandler(*handler.CommandEvent) error {
	return nil
}

func TestSlashCommandBuildsPrimitiveOptions(t *testing.T) {
	command := rave.Slash("note", "Add a note").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("text", "The note text").
				WithRequired(true),
		)

	built := command.Build()

	require.Equal(t, "note", built.CommandName())
	slash, ok := built.(discord.SlashCommandCreate)
	require.True(t, ok)
	require.Equal(t, "Add a note", slash.Description)
	require.Equal(t, []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionString{
			Name:        "text",
			Description: "The note text",
			Required:    true,
		},
	}, slash.Options)
}

func TestSlashCommandBuildsDirectSubcommand(t *testing.T) {
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("info", "Show configuration").
				Handle(noopCommandHandler).
				AddOptions(rave.OptionBool("verbose", "Show extra details")),
		)

	slash := command.Build().(discord.SlashCommandCreate)

	require.Equal(t, []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "info",
			Description: "Show configuration",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "verbose",
					Description: "Show extra details",
				},
			},
		},
	}, slash.Options)
}

func TestSlashCommandBuildsGroupedSubcommand(t *testing.T) {
	command := rave.Slash("role", "Role commands").
		AddOptions(
			rave.SubCommandGroup("member", "Member role commands").
				AddOptions(
					rave.SubCommand("add", "Add a role").
						Handle(noopCommandHandler).
						AddOptions(rave.OptionRole("role", "Role to add")),
				),
		)

	slash := command.Build().(discord.SlashCommandCreate)

	require.Equal(t, []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommandGroup{
			Name:        "member",
			Description: "Member role commands",
			Options: []discord.ApplicationCommandOptionSubCommand{
				{
					Name:        "add",
					Description: "Add a role",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionRole{
							Name:        "role",
							Description: "Role to add",
						},
					},
				},
			},
		},
	}, slash.Options)
}

func TestSlashCommandBuildRejectsDuplicateOptionNames(t *testing.T) {
	command := rave.Slash("search", "Search messages").
		AddOptions(rave.OptionString("query", "First query")).
		AddOptions(rave.OptionString("query", "Second query"))

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsMixedPrimitiveAndSubcommandOptions(t *testing.T) {
	command := rave.Slash("search", "Search messages").
		AddOptions(rave.OptionString("query", "Search query")).
		AddOptions(
			rave.SubCommand("recent", "Search recent messages").
				Handle(noopCommandHandler),
		)

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsRequiredOptionAfterOptionalOption(t *testing.T) {
	command := rave.Slash("search", "Search messages").
		AddOptions(rave.OptionString("channel", "Optional channel")).
		AddOptions(
			rave.OptionString("query", "Required query").
				WithRequired(true),
		)

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsNestedSubcommand(t *testing.T) {
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("parent", "Invalid parent").
				AddOptions(
					rave.SubCommand("child", "Invalid child").
						Handle(noopCommandHandler),
				),
		)

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsInvalidRootDefinition(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		description string
	}{
		{name: "invalid name", commandName: "Not-Lowercase", description: "Description"},
		{name: "empty description", commandName: "valid", description: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := rave.Slash(tt.commandName, tt.description)

			require.Panics(t, func() {
				command.Build()
			})
		})
	}
}

func TestSubcommandsDoNotExposeRequiredOptionMethod(t *testing.T) {
	_, subCommandHasRequired := reflect.TypeOf(
		rave.SubCommand("child", "Child command"),
	).MethodByName("WithRequired")
	_, groupHasRequired := reflect.TypeOf(
		rave.SubCommandGroup("group", "Command group"),
	).MethodByName("WithRequired")

	require.False(t, subCommandHasRequired)
	require.False(t, groupHasRequired)
}

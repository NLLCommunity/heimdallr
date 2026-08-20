package kick

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/require"
)

func TestKickWithMessageRequiresUserAndMessage(t *testing.T) {
	command := KickWithMessage.Build().(discord.SlashCommandCreate)

	var withMessage discord.ApplicationCommandOptionSubCommand
	foundSubcommand := false
	for _, option := range command.Options {
		subcommand, ok := option.(discord.ApplicationCommandOptionSubCommand)
		if ok && subcommand.Name == "with-message" {
			withMessage = subcommand
			foundSubcommand = true
			break
		}
	}
	require.True(t, foundSubcommand)

	var user discord.ApplicationCommandOptionUser
	var message discord.ApplicationCommandOptionString
	foundUser := false
	foundMessage := false
	for _, option := range withMessage.Options {
		switch typed := option.(type) {
		case discord.ApplicationCommandOptionUser:
			if typed.Name == "user" {
				user = typed
				foundUser = true
			}
		case discord.ApplicationCommandOptionString:
			if typed.Name == "message" {
				message = typed
				foundMessage = true
			}
		}
	}

	require.True(t, foundUser)
	require.True(t, user.Required)
	require.True(t, foundMessage)
	require.True(t, message.Required)
}

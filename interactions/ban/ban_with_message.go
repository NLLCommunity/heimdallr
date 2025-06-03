package ban

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

var banWithMessageSubCommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "with-message",
	Description: "Ban a user, sending a message immediately before the ban",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to ban",
			Required:    true,
		},
		discord.ApplicationCommandOptionString{
			Name:        "message",
			Description: "The message to give the user before banning them (also used as ban reason)",
			Required:    true,
		},
	},
}

func BanWithMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ban", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	data := e.SlashCommandInteractionData()
	user := data.User("user")
	banningUser := e.User()
	message := data.String("message")

	banData := BanHandlerData{
		User:          &user,
		BanningUserID: banningUser.ID,
		BanningUser:   &banningUser,
		Guild:         &guild,
		Duration:      "",
		Reason:        message,
		Message:       message,
	}
	return banHandlerInner(e, banData)
}

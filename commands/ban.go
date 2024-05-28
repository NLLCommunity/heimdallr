package commands

import (
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/myrkvi/heimdallr/utils"
)

var BanCommand = discord.SlashCommandCreate{
	Name:                     "ban",
	Description:              "Ban a user from the server",
	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionBanMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
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
					Description: "The message to give the user before banning them",
					Required:    true,
				},
			},
		},
	},
}

func BanWithMessageHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}
	user := data.User("user")
	message := data.String("message")

	mc := discord.NewMessageCreateBuilder().
		SetContentf(
			"You have been banned from %s.\n"+
				"Along with the ban, this message was added:\n\n %s\n\n"+
				"(You cannot respond to this message.)",
			guild.Name,
			message,
		).Build()

	failedToMessage := false
	_, err := SendDirectMessage(e.Client(), user, mc)
	if err != nil {
		failedToMessage = true
	}

	err = e.Client().Rest().AddBan(guild.ID, user.ID, 0,
		rest.WithReason(fmt.Sprintf("Banned by: %s (%s), with message: %s", e.User().Username, e.User().ID, message)))
	if err != nil {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Failed to ban user").
				Build())
	}

	if failedToMessage {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("User was banned but message failed to send.").
				Build())
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("User was banned.").
			Build())

}

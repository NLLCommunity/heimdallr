package commands

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/myrkvi/heimdallr/utils"
)

var KickCommand = discord.SlashCommandCreate{
	Name:                     "kick",
	Description:              "Kick a user from the server",
	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "with-message",
			Description: "Kick a user, sending a message immediately before the kick",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionUser{
					Name:        "user",
					Description: "The user to kick",
					Required:    true,
				},
				discord.ApplicationCommandOptionString{
					Name:        "message",
					Description: "The message to give the user before kicking them",
					Required:    true,
				},
			},
		},
	},
}

func KickWithMessageHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	user := data.User("user")
	message := data.String("message")

	mc := discord.NewMessageCreateBuilder().
		SetContentf(
			"You have been kicked from %s.\n"+
				"Additionally, this message was added:\n\n%s\n\n"+
				"(You cannot respond to this message.)",
			guild.Name,
			message,
		).Build()

	failedToMessage := false
	_, err := SendDirectMessage(e.Client(), user, mc)
	if err != nil {
		failedToMessage = true
	}

	err = e.Client().Rest().RemoveMember(guild.ID, user.ID,
		rest.WithReason(fmt.Sprintf("Kicked by: %s (%s), with message: %s", e.User().Username, e.User().ID, message)))
	if err != nil {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContentf("Failed to kick user %s.", user.Mention()).
				SetAllowedMentions(&discord.AllowedMentions{
					RepliedUser: true,
				}).Build())
	}

	if failedToMessage {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("User was kicked but message failed to send.").
				Build())
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContentf("User %s was kicked.", user.Mention()).
			SetAllowedMentions(&discord.AllowedMentions{
				RepliedUser: true,
			}).Build())
}

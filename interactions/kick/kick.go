package kick

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Route(
		"/kick", func(r handler.Router) {
			r.Command("/with-message", KickWithMessageHandler)
		},
	)

	return []discord.ApplicationCommandCreate{KickCommand}
}

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
	utils.LogInteraction("kick with-message", e)

	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
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
	_, err := interactions.SendDirectMessage(e.Client(), user, mc)
	if err != nil {
		failedToMessage = true
	}

	err = e.Client().Rest().RemoveMember(
		guild.ID, user.ID,
		rest.WithReason(fmt.Sprintf("Kicked by: %s (%s), with message: %s", e.User().Username, e.User().ID, message)),
	)
	if err != nil {
		return interactions.RespondWithContentEph(e, "Failed to kick user %s.", user.Mention())
	}

	if failedToMessage {
		return interactions.RespondWithContentEph(e, "User was kicked but message failed to send.")
	}

	return interactions.RespondWithContentEph(e, "User %s was kicked.", user.Mention())
}

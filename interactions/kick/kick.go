package kick

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r handler.Router) []discord.ApplicationCommandCreate {
	slash := KickWithMessage.Register(r)

	return []discord.ApplicationCommandCreate{slash}
}

var KickWithMessage = rave.Slash("kick", "Kick a user from the server").
	AddNameLocalization(discord.LocaleNorwegian, "spark-ut").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Spark en bruker ut av serveren").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionKickMembers).
	AddOptions(
		rave.SubCommand("with-message", "Kick a user, sending a message immediately before the kick").
			AddNameLocalization(discord.LocaleNorwegian, "med-melding").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Spark en bruker ut av serveren, og send en melding til brukeren før sparkingen.").
			AddOptions(
				rave.OptionUser("user", "The user to kick.").
					AddNameLocalization(discord.LocaleNorwegian, "bruker").
					AddDescriptionLocalization(discord.LocaleNorwegian, "Brukeren du vil sparke ut."),
				rave.OptionString("message", "The message to give the user before kicking them.").
					AddNameLocalization(discord.LocaleNorwegian, "melding").
					AddDescriptionLocalization(discord.LocaleNorwegian, "Meldingen som skal sendes til brukeren før sparkingen."),
			).Handle(KickWithMessageHandler),
	)

func KickWithMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("kick", e)

	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	user := data.User("user")
	message := data.String("message")

	mc := discord.NewMessageCreate().
		WithContentf(
			"You have been kicked from %s.\n"+
				"Additionally, this message was added:\n\n%s\n\n"+
				"(You cannot respond to this message.)",
			guild.Name,
			message,
		)

	failedToMessage := false
	_, err := interactions.SendDirectMessage(e.Client(), user, mc)
	if err != nil {
		failedToMessage = true
	}

	err = e.Client().Rest.RemoveMember(
		guild.ID, user.ID,
		rest.WithReason(fmt.Sprintf("Kicked by: %s (%s), with message: %s", e.User().Username, e.User().ID, message)),
	)
	if err != nil {
		return e.CreateMessage(
			interactions.EphemeralMessageContentf(
				"Failed to kick user %s.", user.Mention(),
			),
		)
	}

	if failedToMessage {
		return e.CreateMessage(
			interactions.EphemeralMessageContentf(
				"User was kicked but message failed to send.",
			),
		)
	}

	return e.CreateMessage(
		interactions.EphemeralMessageContentf(
			"User %s was kicked.", user.Mention(),
		),
	)
}

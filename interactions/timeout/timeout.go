package timeout

import (
	"time"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
)

var Interactions = rave.Bundle(Timeout)

var Timeout = rave.Slash("timeout", "Timeout a user from the server").
	AddNameLocalization(discord.LocaleNorwegian, "timeout").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Timeout en bruker fra serveren").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionModerateMembers).
	AddOptions(
		rave.OptionUser("user", "The user to timeout").WithRequired(true).
			AddNameLocalization(discord.LocaleNorwegian, "bruker").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Brukeren du vil gi timeout til."),
		rave.OptionString("duration", "The duration to timeout the user for (format: 3w2d1h4m28s)").WithRequired(true).
			AddNameLocalization(discord.LocaleNorwegian, "varighet").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Varigheten for timeouten (format: 3w2d1h4m28s)"),
		rave.OptionString("reason", "Reason for timing out the user. Not sent to the user.").WithRequired(false).
			AddNameLocalization(discord.LocaleNorwegian, "årsak").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Årsaken til timeouten. Ikke sendt til brukeren."),
	).Handle(TimeoutHandler)

func TimeoutHandler(e *handler.CommandEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	data := e.SlashCommandInteractionData()
	user := data.Member("user")
	durationStr := data.String("duration")
	reason, hasReason := data.OptString("reason")

	duration, err := utils.ParseLongDuration(durationStr)
	if err != nil {
		return e.CreateMessage(interactions.EphemeralMessageContent("Invalid duration format. Please use the format: 3w2d1h4m28s"))
	}

	if duration < time.Second {
		return e.CreateMessage(interactions.EphemeralMessageContent("Duration must be at least 1 second."))
	} else if duration > 28*24*time.Hour {
		return e.CreateMessage(interactions.EphemeralMessageContent("Duration must be less than 28 days."))
	}

	_, err = e.Client().Rest.UpdateMember(guild.ID, user.User.ID,
		discord.MemberUpdate{
			CommunicationDisabledUntil: omit.NewPtr(time.Now().Add(duration)),
		},
		rest.WithReason(utils.Iif(hasReason, reason, "No reason provided.")),
	)

	if err != nil {
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to timeout user: " + err.Error()))
	}

	return e.CreateMessage(interactions.EphemeralMessageContentf(
		"User %s has been timed out for %s.",
		user.User.Username,
		utils.DurationToHumanReadable(duration),
	))
}

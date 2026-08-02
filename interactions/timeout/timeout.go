package timeout

import (
	"time"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/timeout", TimeoutHandler)
	return []discord.ApplicationCommandCreate{TimeoutCommand}
}

var TimeoutCommand = discord.SlashCommandCreate{
	Name:                     "timeout",
	Description:              "Timeout a user from the server",
	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
	DefaultMemberPermissions: omit.NewPtr(discord.PermissionModerateMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to timeout",
			Required:    true,
		},
		discord.ApplicationCommandOptionString{
			Name:        "duration",
			Description: "The duration to timeout the user for (format: 3w2d1h4m28s)",
			Required:    true,
		},
		discord.ApplicationCommandOptionString{
			Name:        "reason",
			Description: "Reason for timing out the user. Not sent to the user.",
			Required:    false,
		},
	},
}

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

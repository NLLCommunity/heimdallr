package timeout

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"
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
			WithMaxLength(512).
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

	invoker := e.Member()
	var roles []discord.Role
	for role := range e.Client().Caches.Roles(guild.ID) {
		roles = append(roles, role)
	}
	if invoker == nil || !canTimeout(*invoker, user, guild, roles) {
		return e.CreateMessage(interactions.EphemeralMessageContent("You cannot timeout this user."))
	}
	if utf8.RuneCountInString(reason) > 512 {
		return e.CreateMessage(interactions.EphemeralMessageContent("Reason must be 512 characters or fewer."))
	}

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
		responseErr := e.CreateMessage(interactions.EphemeralMessageContent("Failed to timeout user."))
		return errors.Join(err, responseErr)
	}

	return e.CreateMessage(interactions.EphemeralMessageContentf(
		"User %s has been timed out for %s.",
		user.User.Username,
		utils.DurationToHumanReadable(duration),
	))
}

func canTimeout(invoker, target discord.ResolvedMember, guild discord.Guild, roles []discord.Role) bool {
	if !invoker.Permissions.Has(discord.PermissionModerateMembers) ||
		invoker.User.ID == target.User.ID ||
		guild.OwnerID == target.User.ID ||
		target.Permissions.Has(discord.PermissionAdministrator) {
		return false
	}

	roleByID := make(map[snowflake.ID]discord.Role, len(roles))
	for _, role := range roles {
		roleByID[role.ID] = role
	}
	for _, roleID := range target.RoleIDs {
		if role, ok := roleByID[roleID]; ok && role.Permissions.Has(discord.PermissionAdministrator) {
			return false
		}
	}

	highestPosition := func(member discord.ResolvedMember) int {
		highest := 0
		for _, roleID := range member.RoleIDs {
			role, ok := roleByID[roleID]
			if !ok {
				continue
			}
			if role.Position > highest {
				highest = role.Position
			}
		}
		return highest
	}

	return guild.OwnerID == invoker.User.ID || highestPosition(invoker) > highestPosition(target)
}

package infractions

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

var Warn = rave.Slash("warn", "Warn a user").
	AddNameLocalization(discord.LocaleNorwegian, "advar").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Advar en bruker.").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionKickMembers).
	AddOptions(
		rave.OptionUser("user", "The user to warn").
			AddNameLocalization(discord.LocaleNorwegian, "bruker").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Brukeren du vil advare.").
			WithRequired(true),

		rave.OptionString("reason", "The reason for the warning").
			AddNameLocalization(discord.LocaleNorwegian, "aarsak").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Årsaken til advarselen.").
			WithRequired(true),

		rave.OptionFloat("severity", "The severity of the warning").
			AddNameLocalization(discord.LocaleNorwegian, "alvorlighet").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Alvorlighetsgraden til advarselen.").
			WithRequired(false).
			WithMinValue(0.0).
			WithMaxValue(10.0),

		rave.OptionBool("silent", "Whether the warning should be silent / logged without messaging the user").
			AddNameLocalization(discord.LocaleNorwegian, "stille").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Om advarselen skal være stille / lagres uten å varsle brukeren").
			WithRequired(false),
	).
	Handle(WarnHandler)

func WarnHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("infractions", e)

	ctx := context.Background()
	data := e.SlashCommandInteractionData()

	user := data.User("user")
	reason := data.String("reason")
	severity, severityIsSet := data.OptFloat("severity")
	silent, silentIsSet := data.OptBool("silent")

	if !severityIsSet {
		severity = 1.0
	}
	if !silentIsSet {
		silent = false
	}

	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return interactions.ErrEventNoGuildID
	}

	slog.DebugContext(
		ctx, "Received /warn command.",
		"user", user.Username,
		"guild", guild.Name,
		"moderator", e.User().Username,
	)

	inf, err := model.CreateInfraction(guild.ID, user.ID, e.User().ID, reason, severity, silent)
	if err != nil {
		_ = e.CreateMessage(
			interactions.EphemeralMessageContent(
				"Failed to create infraction.",
			),
		)
		return fmt.Errorf("failed to create infraction: %w", err)
	}

	slog.DebugContext(ctx, "Created infraction.", "infraction", inf.Sqid())

	moderatorID := e.User().ID
	targetID := user.ID
	audit.Log(audit.Entry{
		GuildID:    guild.ID,
		EventType:  audit.EventBotWarn,
		ActorID:    &moderatorID,
		ActorKind:  audit.ActorUser,
		TargetID:   &targetID,
		TargetKind: audit.TargetUser,
		Source:     audit.SourceCommand,
		Reason:     inf.Reason,
		Details: map[string]any{
			"infraction_id":   inf.Sqid(),
			"weight":          inf.Weight,
			"silent":          inf.Silent,
			"actor_username":  e.User().Username,
			"target_username": user.Username,
		},
	})

	embed := discord.NewEmbed().
		WithTitlef(`Warning in "%s"`, guild.Name).
		WithDescription(inf.Reason).
		WithColor(severityToColor(inf.Weight)).
		WithTimestamp(inf.Timestamp)

	slog.DebugContext(ctx, "Created embed.")

	failedToSend := false
	if !inf.Silent {
		channel, err := e.Client().Rest.CreateDMChannel(user.ID)
		if err != nil || channel == nil {
			failedToSend = true
		} else {

			_, err = e.Client().Rest.CreateMessage(
				channel.ID(), discord.NewMessageCreate().
					WithEmbeds(embed),
			)
			if err != nil {
				failedToSend = true
			}
		}
	}

	message := discord.NewMessageCreate().
		WithContentf(
			"## %s %s.",
			utils.Iif(
				inf.Silent, "Silent warning created for",
				utils.Iif(failedToSend, "Failed to send warning to", "Warning sent to"),
			),
			user.Mention(),
		).
		WithEmbeds(embed)

	guildSettings, err := model.GetGuildSettings(guild.ID)
	if err == nil && guildSettings.ModeratorChannel != 0 {
		_, err = e.Client().Rest.CreateMessage(guildSettings.ModeratorChannel, message)
		if err != nil {
			slog.Error(
				"Failed to send warning to moderator channel.",
				"err", err,
				"guildID", guild.ID,
				"channelID", guildSettings.ModeratorChannel,
				"userID", user.ID,
			)
		}
	}

	return e.CreateMessage(
		interactions.EphemeralMessageContentf(
			"Warning created for %s.", user.Mention(),
		),
	)
}

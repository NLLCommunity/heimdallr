package infractions

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

// WarnCommand lets moderators issue warnings to users.
var WarnCommand = discord.SlashCommandCreate{
	Name: "warn",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "advar",
	},
	Description: "Warn a user.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Advar en bruker.",
	},

	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name: "user",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "bruker",
			},
			Description: "The user to warn.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Brukeren du vil advare.",
			},
			Required: true,
		},

		discord.ApplicationCommandOptionString{
			Name: "reason",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "aarsak",
			},
			Description: "The reason for the warning.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Årsaken til advarselen.",
			},
			Required: true,
		},

		discord.ApplicationCommandOptionFloat{
			Name: "severity",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "alvorlighet",
			},
			Description: "The severity of the warning.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Alvorlighetsgraden til advarselen.",
			},
			Required: false,
			MinValue: utils.Ref(0.0),
			MaxValue: utils.Ref(10.0),
		},

		discord.ApplicationCommandOptionBool{
			Name: "silent",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "stille",
			},
			Description: "Whether the warning should be silent / logged without messaging the user",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Om advarselen skal være stille / lagres uten å varsle brukeren",
			},
			Required: false,
		},
	},
}

func WarnHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("warn", e)

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
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Failed to create infraction.").Build(),
		)
		return fmt.Errorf("failed to create infraction: %w", err)
	}

	slog.DebugContext(ctx, "Created infraction.", "infraction", inf.Sqid())

	embed := discord.NewEmbedBuilder().
		SetTitlef(`Warning in "%s"`, guild.Name).
		SetDescription(inf.Reason).
		SetColor(severityToColor(inf.Weight)).
		SetTimestamp(inf.Timestamp)

	slog.DebugContext(ctx, "Created embed.")

	failedToSend := false
	if !inf.Silent {
		channel, err := e.Client().Rest().CreateDMChannel(user.ID)
		if err != nil {
			failedToSend = true
		}
		_, err = e.Client().Rest().CreateMessage(
			channel.ID(), discord.NewMessageCreateBuilder().
				SetEmbeds(embed.Build()).
				Build(),
		)
		if err != nil {
			failedToSend = true
		}
	}

	message := discord.NewMessageCreateBuilder().
		SetContentf(
			"## %s %s.",
			utils.Iif(
				inf.Silent, "Silent warning created for",
				utils.Iif(failedToSend, "Failed to send warning to", "Warning sent to"),
			),
			user.Mention(),
		).
		SetEmbeds(embed.Build()).
		Build()

	guildSettings, err := model.GetGuildSettings(guild.ID)
	if err == nil && guildSettings.ModeratorChannel != 0 {
		_, err = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, message)
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
		discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContentf(
				"Warning created for %s.",
				user.Mention(),
			).Build(),
	)
}

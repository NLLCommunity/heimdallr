package commands

import (
	"context"
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
	"log/slog"
	"math"
	"strconv"
	"time"
)

// pageSize is the size of one page of infractions
const pageSize = 5

//░█░█░█▀█░█▀▄░█▀█░░░█▀▀░█▀█░█▄█░█▄█░█▀█░█▀█░█▀▄
//░█▄█░█▀█░█▀▄░█░█░░░█░░░█░█░█░█░█░█░█▀█░█░█░█░█
//░▀░▀░▀░▀░▀░▀░▀░▀░░░▀▀▀░▀▀▀░▀░▀░▀░▀░▀░▀░▀░▀░▀▀░

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
		return ErrEventNoGuildID
	}

	slog.DebugContext(ctx, "Received /warn command.",
		"user", user.Username,
		"guild", guild.Name,
		"moderator", e.User().Username)

	inf, err := model.CreateInfraction(guild.ID, user.ID, e.User().ID, reason, float32(severity), silent)
	if err != nil {
		return fmt.Errorf("failed to create infraction: %w", err)
	}

	slog.DebugContext(ctx, "Created infraction.", "infraction", inf.Sqid())

	if inf.Silent {
		err = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(fmt.Sprintf("A silent warning has been added for %s", user.Mention())).Build())
		if err != nil {
			slog.Error("Failed to respond to 'warn' command interaction.",
				"err", err,
				"guildID", guild.ID,
				"userID", user.ID)
			return err
		}
		return nil
	}

	embed := discord.NewEmbedBuilder().
		SetTitlef(`Warning in "%s"`, guild.Name).
		SetDescription(inf.Reason).
		SetColor(severityToColor(inf.Weight)).
		SetTimestamp(inf.Timestamp)

	slog.DebugContext(ctx, "Created embed.")

	channel, err := e.Client().Rest().CreateDMChannel(user.ID)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent(fmt.Sprintf("Failed to send warning to %s.", user.Mention())).Build())
	}
	_, err = e.Client().Rest().CreateMessage(channel.ID(), discord.NewMessageCreateBuilder().
		SetEmbeds(embed.Build()).
		Build())
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent(fmt.Sprintf("Failed to send warning to %s.", user.Mention())).Build())
	}

	guildSettings, err := model.GetGuildSettings(guild.ID)
	if err == nil && guildSettings.ModeratorChannel != 0 {
		_, err = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
			SetContent(fmt.Sprintf("## Warning sent to %s.", user.Mention())).
			SetEmbeds(embed.Build()).
			Build())
		if err != nil {
			slog.Error("Failed to send warning to moderator channel.",
				"err", err,
				"guildID", guild.ID,
				"channelID", guildSettings.ModeratorChannel,
				"userID", user.ID)
		}
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Warning sent to %s.", user.Mention())).Build())
}

func severityToColor(severity float32) int {
	if severity >= 3.0 {
		return 0xFF0000
	}
	if severity >= 1.0 {
		return 0xFF9100
	}
	return 0xFFFF00
}

// ░█░█░█▀▀░█▀▀░█▀▄░░░▀█▀░█▀█░█▀▀░█▀▄░█▀█░█▀▀░▀█▀░▀█▀░█▀█░█▀█░█▀▀
// ░█░█░▀▀█░█▀▀░█▀▄░░░░█░░█░█░█▀▀░█▀▄░█▀█░█░░░░█░░░█░░█░█░█░█░▀▀█
// ░▀▀▀░▀▀▀░▀▀▀░▀░▀░░░▀▀▀░▀░▀░▀░░░▀░▀░▀░▀░▀▀▀░░▀░░▀▀▀░▀▀▀░▀░▀░▀▀▀

// UserInfractionsCommand lets users view their own infractions.
var UserInfractionsCommand = discord.SlashCommandCreate{
	Name: "warnings",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "advarsler",
	},
	Description: "View your warnings.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Se advarslene dine.",
	},

	DMPermission: utils.Ref(false),
}

func UserInfractionsHandler(e *handler.CommandEvent) error {
	user := e.User()
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return ErrEventNoGuildID
	}
	infrData, err := getUserInfractions(guild.ID, user.ID, pageSize, 0)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to retrieve infractions.").
			Build())
	}

	if infrData.TotalCount == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("You have no infractions.").
			SetEphemeral(true).
			Build())
	}

	message := discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetEmbeds(infrData.Embeds...).
		SetContentf("You have %d infractions. (Viewing %d-%d)\nTotal severity: %.2f",
			infrData.TotalCount,
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			infrData.TotalSeverity,
		)

	if infrData.Components != nil {
		message.AddActionRow(infrData.Components...)
	}

	return e.CreateMessage(message.Build())
}

func UserInfractionButtonHandler(e *handler.ComponentEvent) error {
	offsetStr := e.Variables["offset"]
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return fmt.Errorf("failed to parse offset: %w", err)
	}

	user := e.User()
	guild, ok := e.Guild()
	if !ok {
		return ErrEventNoGuildID
	}

	infrData, err := getUserInfractions(guild.ID, user.ID, pageSize, offset)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to retrieve infractions.").
			Build())
	}

	if infrData.TotalCount == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("You have no infractions.").
			SetEphemeral(true).
			Build())
	}

	return e.UpdateMessage(discord.NewMessageUpdateBuilder().
		SetEmbeds(infrData.Embeds...).
		AddActionRow(infrData.Components...).
		SetContentf("You have %d infractions. (Viewing %d-%d)\nTotal severity: %.2f",
			infrData.TotalCount,
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			infrData.TotalSeverity).Build())

}

//░▀█▀░█▀█░█▀▀░█▀▄░█▀█░█▀▀░▀█▀░▀█▀░█▀█░█▀█░█▀▀░░░▄▀░░█▄█░█▀█░█▀▄░▀▄░
//░░█░░█░█░█▀▀░█▀▄░█▀█░█░░░░█░░░█░░█░█░█░█░▀▀█░░░█░░░█░█░█░█░█░█░░█░
//░▀▀▀░▀░▀░▀░░░▀░▀░▀░▀░▀▀▀░░▀░░▀▀▀░▀▀▀░▀░▀░▀▀▀░░░░▀░░▀░▀░▀▀▀░▀▀░░▀░░

// InfractionsCommand is a set of subcommands to manage infractions.
var InfractionsCommand = discord.SlashCommandCreate{
	Name: "infractions",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "advarsler",
	},
	Description: "View a user's warnings.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Se en brukers advarsler.",
	},

	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name: "list",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "liste",
			},
			Description: "View a user's warnings. (NB: Response visible to all)",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Se en brukers advarsler.",
			},
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionUser{
					Name: "user",
					NameLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "bruker",
					},
					Description: "The user to view warnings for.",
					DescriptionLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "Brukeren du vil se advarsler for.",
					},
					Required: true,
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name: "remove",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "fjern",
			},
			Description: "Remove a user's warning.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Fjern en brukers advarsel.",
			},
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name: "infraction-id",
					NameLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "advarsels-id",
					},
					Description: "The id of the infraction to remove.",
					DescriptionLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "ID-en til advarselen du vil fjerne.",
					},
					Required: true,
				},
			},
		},
	},
}

// InfractionsListHandler handles the `/infractions list` command.
func InfractionsListHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	user := data.User("user")
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return ErrEventNoGuildID
	}

	infrData, err := getUserInfractions(guild.ID, user.ID, pageSize, 0)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to retrieve infractions.").
			Build())
	}

	if infrData.TotalCount == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetEphemeral(true).
			SetContentf("%s has no infractions.", user.Mention()).
			Build())
	}

	message := discord.NewMessageCreateBuilder().
		SetAllowedMentions(&discord.AllowedMentions{}).
		SetEmbeds(infrData.Embeds...).
		SetContentf("%s has %d infractions. (Viewing %d-%d)\nTotal severity: %.2f",
			user.Mention(),
			infrData.TotalCount,
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			infrData.TotalSeverity)

	if infrData.Components != nil {
		message.AddActionRow(infrData.Components...)
	}

	return e.CreateMessage(message.Build())
}

func InfractionsRemoveHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	infID := data.String("infraction-id")
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return ErrEventNoGuildID
	}

	err := model.DeleteInfractionBySqid(infID)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to delete infraction.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent("Infraction deleted.").
		Build())
}

func InfractionsListComponentHandler(e *handler.ComponentEvent) error {
	parentIx := e.Message.Interaction
	if parentIx == nil {
		return fmt.Errorf("no parent interaction found")
	}

	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}
	offset, err := strconv.Atoi(e.Variables["offset"])
	if err != nil {
		return fmt.Errorf("failed to parse offset: %w", err)
	}
	userID, err := snowflake.Parse(e.Variables["userID"])
	if err != nil {
		return fmt.Errorf("failed to parse user id: %w", err)
	}

	user, err := e.Client().Rest().GetUser(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if e.User().ID != parentIx.User.ID {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetContent("You can only paginate responses from your own commands.").
			SetEphemeral(true).
			Build())
	}

	infrData, err := getUserInfractions(guild.ID, userID, pageSize, offset)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("Failed to retrieve infractions.").
			SetEphemeral(true).
			Build())
	}

	if infrData.TotalCount == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetContentf("%s has no infractions. You shouldn't be able to navigate to this, though?", user.Mention()).
			SetEphemeral(true).
			Build())
	}

	if infrData.CurrentCount == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("No more infractions to show.").
			SetEphemeral(true).
			Build())
	}

	message := discord.NewMessageUpdateBuilder().
		SetAllowedMentions(&discord.AllowedMentions{}).
		ClearEmbeds().
		ClearContainerComponents().
		SetEmbeds(infrData.Embeds...).
		SetContentf("%s has %d infractions. (Viewing %d-%d)\nTotal severity: %.2f",
			user.Mention(),
			infrData.TotalCount,
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			infrData.TotalSeverity).
		AddActionRow(infrData.Components...)

	return e.UpdateMessage(message.Build())
}

//░█░█░▀█▀░▀█▀░█░░░▀█▀░▀█▀░█░█░░░█▀▀░▀█▀░█▀▄░█░█░█▀▀░▀█▀░█▀▀░░░█░█▀▀░█░█░█▀█░█▀▀░█▀▀
//░█░█░░█░░░█░░█░░░░█░░░█░░░█░░░░▀▀█░░█░░█▀▄░█░█░█░░░░█░░▀▀█░▄▀░░█▀▀░█░█░█░█░█░░░▀▀█
//░▀▀▀░░▀░░▀▀▀░▀▀▀░▀▀▀░░▀░░░▀░░░░▀▀▀░░▀░░▀░▀░▀▀▀░▀▀▀░░▀░░▀▀▀░▀░░░▀░░░▀▀▀░▀░▀░▀▀▀░▀▀▀

type userInfractions struct {
	Embeds        []discord.Embed
	Components    []discord.InteractiveComponent
	TotalCount    int64
	CurrentCount  int
	Offset        int
	TotalSeverity float64
}

func getUserInfractions(guildID, userID snowflake.ID, limit, offset int) (userInfractions, error) {
	infractions, count, err := model.GetUserInfractions(guildID, userID, pageSize, offset)
	if err != nil {
		return userInfractions{}, fmt.Errorf("failed to get user infractions: %w", err)
	}

	if count == 0 {
		return userInfractions{}, nil
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return userInfractions{}, fmt.Errorf("failed to get guild settings: %w", err)
	}

	allInfractions, _, err := model.GetUserInfractions(guildID, userID, math.MaxInt, 0)
	if err != nil {
		return userInfractions{}, fmt.Errorf("failed to get all user infractions: %w", err)
	}

	severity := 0.0
	for _, inf := range allInfractions {
		diff := time.Since(inf.CreatedAt)
		severity += utils.CalcHalfLife(diff, guildSettings.InfractionHalfLifeDays, float64(inf.Weight))
	}

	embeds := createInfractionEmbeds(infractions)

	var components []discord.InteractiveComponent
	slog.Info("Count is", "count", count)
	if count > pageSize {
		slog.Debug("Adding action row buttons.")
		if offset > 0 {
			components = append(components, discord.NewPrimaryButton("Previous",
				fmt.Sprintf("/infractions-mod/%s/%d",
					userID,
					utils.Max(0, offset-pageSize)),
			))
		} else {
			components = append(components, discord.NewPrimaryButton("Previous", "unreachable").
				AsDisabled())
		}
		if count > int64(offset+pageSize) {
			components = append(components, discord.NewPrimaryButton("Next",
				fmt.Sprintf("/infractions-mod/%s/%d",
					userID,
					utils.Min(count-1, int64(offset+pageSize)),
				)))
		} else {
			components = append(components, discord.NewPrimaryButton("Next", "unreachable").
				AsDisabled())
		}
	}

	return userInfractions{
		Embeds:        embeds,
		Components:    components,
		TotalCount:    count,
		CurrentCount:  len(embeds),
		Offset:        offset,
		TotalSeverity: severity,
	}, nil
}

func createInfractionEmbeds(infractions []model.Infraction) []discord.Embed {
	var embeds []discord.Embed

	for _, inf := range infractions {
		embed := discord.NewEmbedBuilder().
			SetTitlef("Infraction `%s`", inf.Sqid()).
			SetDescription(inf.Reason).
			SetColor(severityToColor(inf.Weight)).
			SetTimestamp(inf.Timestamp).
			AddField("Severity", fmt.Sprintf("%.4g", inf.Weight), true)

		embeds = append(embeds, embed.Build())
	}
	return embeds
}

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
	"strings"
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

	inf, err := model.CreateInfraction(guild.ID, user.ID, e.User().ID, reason, severity, silent)
	if err != nil {
		return fmt.Errorf("failed to create infraction: %w", err)
	}

	slog.DebugContext(ctx, "Created infraction.", "infraction", inf.Sqid())

	if inf.Silent {
		err = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
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

func severityToColor(severity float64) int {
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

	message, err := getUserInfractionsAndMakeMessage(false, &guild, &user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
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

	mcb, mub, err := getUserInfractionsAndUpdateMessage(false, offset, &guild, &user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}
	if mcb != nil {
		return e.CreateMessage(mcb.Build())
	}
	return e.UpdateMessage(mub.Build())
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
					Required: false,
				},
				discord.ApplicationCommandOptionString{
					Name: "user-id",
					NameLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "bruker-id",
					},
					Description: "The ID of the user user to view warnings for.",
					DescriptionLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "ID-en til brukeren du vil se advarsler for.",
					},
					Required: false,
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
	user, hasUser := data.OptUser("user")
	userIDString, hasUserID := data.OptString("user-id")
	userID, err := snowflake.Parse(userIDString)
	if err != nil {
		return fmt.Errorf("failed to parse user id: %w", err)
	}

	if !hasUser && !hasUserID {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("You must specify either a user or a user ID.").
			SetEphemeral(true).
			Build())
	}

	if hasUser && hasUserID {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("You can only specify either a user or a user ID.").
			SetEphemeral(true).
			Build())
	}

	if !hasUser {
		userRef, err := e.Client().Rest().GetUser(userID)
		if err != nil || userRef == nil {
			user = discord.User{
				ID:       userID,
				Username: "unknown_user",
			}
		} else {
			user = *userRef
		}
	}
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return ErrEventNoGuildID
	}

	message, err := getUserInfractionsAndMakeMessage(true, &guild, &user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
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

	mcb, mub, err := getUserInfractionsAndUpdateMessage(false, offset, &guild, user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}
	if mcb != nil {
		return e.CreateMessage(mcb.Build())
	}
	return e.UpdateMessage(mub.Build())
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
		severity += utils.CalcHalfLife(diff, guildSettings.InfractionHalfLifeDays, inf.Weight)
	}

	embeds := createInfractionEmbeds(infractions, guildSettings)

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

func createInfractionEmbeds(infractions []model.Infraction, guildSettings *model.GuildSettings) []discord.Embed {
	var embeds []discord.Embed

	for _, inf := range infractions {
		weightWithHalfLife := utils.CalcHalfLife(
			time.Since(inf.CreatedAt),
			utils.Iif(guildSettings == nil, 0.0, guildSettings.InfractionHalfLifeDays),
			inf.Weight)

		embed := discord.NewEmbedBuilder().
			SetTitlef("Infraction `%s`", inf.Sqid()).
			SetDescription(inf.Reason).
			SetColor(severityToColor(inf.Weight)).
			SetTimestamp(inf.Timestamp).
			AddField("Strikes",
				fmt.Sprintf("%s (%s)\n(at warn time: %s)",
					severityToDots(weightWithHalfLife),
					utils.FormatFloatUpToPrec(weightWithHalfLife, 2),
					utils.FormatFloatUpToPrec(inf.Weight, 2),
				), true)

		embeds = append(embeds, embed.Build())
	}
	return embeds
}

func severityToDots(severity float64) string {
	severityFloor := math.Floor(severity)

	dots := ""
	severityInt := int(severityFloor)
	dots += strings.Repeat("●", severityInt)

	remaining := severity - severityFloor
	if remaining >= 0.0 && remaining < 0.125 {
		dots += "○"
	} else if remaining >= 0.125 && remaining < 0.375 {
		dots += "◔"
	} else if remaining >= 0.375 && remaining < 0.625 {
		dots += "◑"
	} else if remaining >= 0.625 && remaining < 0.875 {
		dots += "◕"
	} else if remaining >= 0.875 {
		dots += "●"
	}

	if dots == "" {
		dots = "○"
	}

	return dots
}

func getUserInfractionsAndMakeMessage(
	modView bool,
	guild *discord.Guild, user *discord.User,
) (*discord.MessageCreateBuilder, error) {
	infrData, err := getUserInfractions(guild.ID, user.ID, pageSize, 0)
	if err != nil {
		return discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Failed to retrieve infractions."),
			fmt.Errorf("failed to get user infractions: %w", err)
	}

	if infrData.TotalCount == 0 {
		return discord.NewMessageCreateBuilder().
				SetAllowedMentions(&discord.AllowedMentions{}).
				SetEphemeral(true).
				SetContentf("%s has no infractions.", user.Mention()),
			nil
	}

	modText := fmt.Sprintf("%s has %d infractions.",
		user.Mention(),
		infrData.TotalCount)
	userText := fmt.Sprintf("You have %d infractions.",
		infrData.TotalCount)

	message := discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		SetEmbeds(infrData.Embeds...).
		SetContentf("%s (Viewing %d-%d)\nTotal strikes: %s",
			utils.Iif(modView, modText, userText),
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			fmt.Sprintf("%s (%s)",
				severityToDots(infrData.TotalSeverity),
				utils.FormatFloatUpToPrec(infrData.TotalSeverity, 2),
			))

	if infrData.Components != nil {
		message.AddActionRow(infrData.Components...)
	}

	return message, nil
}

func getUserInfractionsAndUpdateMessage(
	modView bool, offset int,
	guild *discord.Guild, user *discord.User,
) (mcb *discord.MessageCreateBuilder, mub *discord.MessageUpdateBuilder, err error) {

	infrData, err := getUserInfractions(guild.ID, user.ID, pageSize, offset)
	if err != nil {
		mcb = discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to retrieve infractions.")
		return
	}

	if infrData.TotalCount == 0 {
		mcb = discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("You have no infractions.").
			SetEphemeral(true)
		return
	}

	modText := fmt.Sprintf("%s has %d infractions.",
		user.Mention(),
		infrData.TotalCount)
	userText := fmt.Sprintf("You have %d infractions.",
		infrData.TotalCount)

	mub = discord.NewMessageUpdateBuilder().
		SetEmbeds(infrData.Embeds...).
		AddActionRow(infrData.Components...).
		SetContentf("%s (Viewing %d-%d)\nTotal strikes: %s",
			utils.Iif(modView, modText, userText),
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			fmt.Sprintf("%s (%s)",
				severityToDots(infrData.TotalSeverity),
				utils.FormatFloatUpToPrec(infrData.TotalSeverity, 2),
			))
	return
}

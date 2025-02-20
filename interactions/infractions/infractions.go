package infractions

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/warn", WarnHandler)
	r.Command("/warnings", UserInfractionsHandler)
	r.Route(
		"/infractions", func(r handler.Router) {
			r.Command("/list", InfractionsListHandler)
			r.Command("/remove", InfractionsRemoveHandler)
		},
	)
	r.Component("/infractions-user/{offset}", UserInfractionButtonHandler)
	r.Component("/infractions-mod/{userID}/{offset}", InfractionsListComponentHandler)

	return []discord.ApplicationCommandCreate{
		InfractionsCommand,
		UserInfractionsCommand,
		WarnCommand,
	}
}

// pageSize is the size of one page of infractions
const pageSize = 5

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
			components = append(
				components, discord.NewPrimaryButton(
					"Previous",
					fmt.Sprintf(
						"/infractions-mod/%s/%d",
						userID,
						max(0, offset-pageSize),
					),
				),
			)
		} else {
			components = append(
				components, discord.NewPrimaryButton("Previous", "unreachable").
					AsDisabled(),
			)
		}
		if count > int64(offset+pageSize) {
			components = append(
				components, discord.NewPrimaryButton(
					"Next",
					fmt.Sprintf(
						"/infractions-mod/%s/%d",
						userID,
						min(count-1, int64(offset+pageSize)),
					),
				),
			)
		} else {
			components = append(
				components, discord.NewPrimaryButton("Next", "unreachable").
					AsDisabled(),
			)
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
			inf.Weight,
		)

		embed := discord.NewEmbedBuilder().
			SetTitlef("Infraction `%s`", inf.Sqid()).
			SetDescription(inf.Reason).
			SetColor(severityToColor(inf.Weight)).
			SetTimestamp(inf.Timestamp).
			AddField(
				"Strikes",
				fmt.Sprintf(
					"%s (%s)\n(at warn time: %s)",
					severityToDots(weightWithHalfLife),
					utils.FormatFloatUpToPrec(weightWithHalfLife, 2),
					utils.FormatFloatUpToPrec(inf.Weight, 2),
				), true,
			)

		embeds = append(embeds, embed.Build())
	}
	return embeds
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

	modText := fmt.Sprintf(
		"%s has %d infractions.",
		user.Mention(),
		infrData.TotalCount,
	)
	userText := fmt.Sprintf(
		"You have %d infractions.",
		infrData.TotalCount,
	)

	message := discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		SetEmbeds(infrData.Embeds...).
		SetContentf(
			"%s (Viewing %d-%d)\nTotal strikes: %s",
			utils.Iif(modView, modText, userText),
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			fmt.Sprintf(
				"%s (%s)",
				severityToDots(infrData.TotalSeverity),
				utils.FormatFloatUpToPrec(infrData.TotalSeverity, 2),
			),
		)

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

	modText := fmt.Sprintf(
		"%s has %d infractions.",
		user.Mention(),
		infrData.TotalCount,
	)
	userText := fmt.Sprintf(
		"You have %d infractions.",
		infrData.TotalCount,
	)

	mub = discord.NewMessageUpdateBuilder().
		SetEmbeds(infrData.Embeds...).
		AddActionRow(infrData.Components...).
		SetContentf(
			"%s (Viewing %d-%d)\nTotal strikes: %s",
			utils.Iif(modView, modText, userText),
			infrData.Offset+1,
			infrData.Offset+infrData.CurrentCount,
			fmt.Sprintf(
				"%s (%s)",
				severityToDots(infrData.TotalSeverity),
				utils.FormatFloatUpToPrec(infrData.TotalSeverity, 2),
			),
		)
	return
}

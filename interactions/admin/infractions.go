package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var infractionsSubCommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "infractions",
	Description: "View or set infraction-related settings",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionFloat{
			Name:        "half-life",
			Description: "The half-life of infractions in days (0 = no half-life)",
			Required:    false,
			MinValue:    utils.Ref(0.0),
			MaxValue:    utils.Ref(365.0),
		},
		discord.ApplicationCommandOptionBool{
			Name:        "notify-warned-user-join",
			Description: "Whether to notify moderator channel when warned user (re)joins the server",
			Required:    false,
		},
		discord.ApplicationCommandOptionFloat{
			Name:        "notify-threshold",
			Description: "The minimum severity of infractions to notify on (0 = always)",
			Required:    false,
			MinValue:    utils.Ref(0.0),
			MaxValue:    utils.Ref(100.0),
		},
	},
}

func AdminInfractionsHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	halfLife, hasHalfLife := data.OptFloat("half-life")
	if hasHalfLife {
		settings.InfractionHalfLifeDays = halfLife
		message += fmt.Sprintf("Infraction half-life set to %.1f days\n", halfLife)
	}

	notifyOnWarnedUserJoin, hasNotifyOnWarnedUserJoin := data.OptBool("notify-warned-user-join")
	if hasNotifyOnWarnedUserJoin {
		settings.NotifyOnWarnedUserJoin = notifyOnWarnedUserJoin
		message += fmt.Sprintf("Notify on warned user join set to %s\n", utils.Iif(notifyOnWarnedUserJoin, "yes", "no"))
	}

	notifyThreshold, hasNotifyThreshold := data.OptFloat("notify-threshold")
	if hasNotifyThreshold {
		settings.NotifyWarnSeverityThreshold = notifyThreshold
		message += fmt.Sprintf("Notify warn severity threshold set to %.1f\n", notifyThreshold)
	}

	if !utils.Any(hasHalfLife, hasNotifyThreshold, hasNotifyOnWarnedUserJoin) {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetContent(infractionInfo(settings)).
				SetEphemeral(true).
				SetAllowedMentions(&discord.AllowedMentions{}).
				Build(),
		)
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetContent(message).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
}

func infractionInfo(settings *model.GuildSettings) string {
	infractionHalfLifeInfo := "> This is the half-life time of infractions' severity in days.\n> A half-life of 0 means that infractions never expire."
	infractionHalfLife := fmt.Sprintf(
		"**Infraction half-life:** %.1f days\n%s",
		settings.InfractionHalfLifeDays, infractionHalfLifeInfo,
	)

	notifyOnWarnedUserJoinInfo := "> This determines whether to notify the moderator channel when a warned user (re)joins the server."
	notifyOnWarnedUserJoin := fmt.Sprintf(
		"**Notify on warned user join:** %s\n%s",
		utils.Iif(settings.NotifyOnWarnedUserJoin, "yes", "no"), notifyOnWarnedUserJoinInfo,
	)

	notifyWarnSeverityThresholdInfo := "> This is the minimum severity of infractions to notify on.\n> A threshold of 0 means that all infractions are notified on."
	notifyWarnSeverityThreshold := fmt.Sprintf(
		"**Notify warn severity threshold:** %.1f\n%s",
		settings.NotifyWarnSeverityThreshold, notifyWarnSeverityThresholdInfo,
	)

	return fmt.Sprintf(
		"## Infraction settings\n%s\n\n%s\n\n%s",
		infractionHalfLife, notifyOnWarnedUserJoin, notifyWarnSeverityThreshold,
	)
}

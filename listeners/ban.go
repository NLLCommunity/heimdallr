package listeners

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	banIx "github.com/NLLCommunity/heimdallr/interactions/ban"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func OnMemberBan(e *events.GuildBan) {
	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	if guildSettings.ModeratorChannel == 0 {
		return
	}

	ban, err := e.Client().Rest().GetBan(e.GuildID, e.User.ID)
	if err != nil || ban == nil {
		_, _ = e.Client().Rest().CreateMessage(
			guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
				SetContentf("User %s (`%d`) was banned", e.User.Username, e.User.ID).
				Build(),
		)
	}

	reason := utils.RefDefault(ban.Reason, "")
	slog.Debug("Parsing ban reason", "reason", reason)
	banData := banIx.BanHandlerDataFromString(reason)

	embed := discord.NewEmbedBuilder().
		SetTitlef("User %s was banned", e.User.EffectiveName()).
		SetDescriptionf(
			"### Reason\n>>> %s",
			utils.Iif(banData.Reason != "", banData.Reason, "none given"),
		).
		SetColor(0xFF0000).
		AddField("Username", e.User.Username, true).
		AddField("User ID", fmt.Sprintf("`%s`", e.User.ID), true)

	if banData.BanningUserID != 0 {
		if banningUser, err := e.Client().Rest().GetUser(banData.BanningUserID); err == nil {
			embed.AddField("Banning User", banningUser.Username, true)
		}
	}
	if banData.Duration != "" {
		embed.AddField("Duration", banData.Duration, true)
	}
	if banData.Message != "" {
		if banData.Message == banData.Reason {
			embed.AddField("Message", "_Sent reason as message, see above._", false)
		} else {
			embed.AddField("Message", banData.Message, false)
		}
	}

	_, _ = e.Client().Rest().CreateMessage(
		guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
			AddEmbeds(embed.Build()).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
}

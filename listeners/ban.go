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

	ban, err := e.Client().Rest.GetBan(e.GuildID, e.User.ID)
	if err != nil || ban == nil {
		_, _ = e.Client().Rest.CreateMessage(
			guildSettings.ModeratorChannel, discord.NewMessageCreate().
				WithContentf("User %s (`%d`) was banned", e.User.Username, e.User.ID).
				WithAllowedMentions(&discord.AllowedMentions{}),
		)
		return
	}

	reason := utils.RefDefault(ban.Reason, "")
	slog.Debug("Parsing ban reason", "reason", reason)
	banData := banIx.BanHandlerDataFromString(reason)

	components := []discord.ContainerSubComponent{
		discord.NewTextDisplayf(
			"## User %s was banned.",
			e.User.EffectiveName(),
		),
		discord.NewTextDisplayf(
			"**Username:** %s\n"+
				"**User ID:** %s\n"+
				"**Banned by:** %s\n"+
				"**Duration:** %s",
			e.User.Username,
			e.User.ID,
			utils.Iif(banData.BanningUserID != 0, fmt.Sprintf("<@%d>", banData.BanningUserID), "unknown"),
			utils.Iif(banData.Duration != "", banData.Duration, "permanent"),
		),
		discord.NewTextDisplayf(
			"### Reason\n>>> %s",
			utils.Iif(banData.Reason != "",
				banData.Reason,
				"none given"),
		),
	}

	if banData.Message != "" {
		if banData.Message == banData.Reason {
			components = append(components, discord.NewTextDisplay("_Sent reason as message, see above._"))
		} else {
			components = append(components, discord.NewTextDisplayf(
				"### Message\n>>> %s",
				banData.Message,
			))
		}
	}

	message := discord.NewMessageCreateV2(
		discord.NewContainer(components...).
			WithAccentColor(0xFF0000),
	).WithAllowedMentions(&discord.AllowedMentions{})

	_, _ = e.Client().Rest.CreateMessage(guildSettings.ModeratorChannel, message)
}

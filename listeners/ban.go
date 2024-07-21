package listeners

import (
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
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
		_, _ = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
			SetContentf("User %s (`%d`) was banned", e.User.Username, e.User.ID).
			Build())
	}

	_, _ = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
		SetContentf("User %s (`%d`) was banned.%s", e.User.Username, e.User.ID,
			utils.Iif(ban.Reason != nil, fmt.Sprintf("\n\n>>> %s", *ban.Reason), "")).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

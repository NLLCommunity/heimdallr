package listeners

import (
	"fmt"
	"math"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func OnWarnedUserJoin(e *events.GuildMemberJoin) {
	fmt.Println("OnWarnedUserJoin called")
	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	if !guildSettings.NotifyOnWarnedUserJoin {
		return
	}

	infractions, _, err := model.GetUserInfractions(e.GuildID, e.Member.User.ID, math.MaxInt, 0)
	if err != nil {
		return
	}

	totalSeverity := 0.0
	for _, infraction := range infractions {
		diff := time.Since(infraction.CreatedAt)
		severity := utils.CalcHalfLife(diff, guildSettings.InfractionHalfLifeDays, float64(infraction.Weight))
		totalSeverity += severity
	}

	if totalSeverity < 1.0 {
		return
	}

	modChannel := guildSettings.ModeratorChannel
	isOwnerChannel := false
	if modChannel == 0 {
		guild, err := e.Client().Rest.GetGuild(e.GuildID, false)
		if err != nil {
			return
		}

		c, err := e.Client().Rest.CreateDMChannel(guild.OwnerID)
		if err != nil {
			return
		}
		modChannel = c.ID()
		isOwnerChannel = true
	}

	extraMsg := ""
	if isOwnerChannel {
		extraMsg = "\n(as the moderator channel has not been set, this message was sent you as the owner of the server)"
	}

	_, _ = e.Client().Rest.CreateMessage(
		modChannel,
		discord.NewMessageCreate().
			WithContentf(
				"%s has joined with a total infraction severity score of %.2f, greater than the threshold of 1.0%s",
				e.Member.Mention(),
				totalSeverity,
				extraMsg,
			),
	)
}

package listeners

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
)

func OnWarnedUserJoin(e *events.GuildMemberJoin) {
	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	if !guildSettings.NotifyOnWarnedUserJoin {
		return
	}

	totalSeverity, err := model.GetUserTotalInfractionWeight(e.GuildID, e.Member.User.ID, guildSettings.InfractionHalfLifeDays)
	if err != nil {
		return
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

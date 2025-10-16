package listeners

import (
	"log/slog"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func OnUserJoin(e *events.GuildMemberJoin) {
	guildID := e.GuildID
	guild, err := e.Client().Rest.GetGuild(guildID, false)
	if err != nil {
		return
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return
	}

	if !guildSettings.JoinMessageEnabled {
		return
	}

	joinLeaveChannel := guildSettings.JoinLeaveChannel
	if joinLeaveChannel == 0 && guild.SystemChannelID != nil {
		joinLeaveChannel = *guild.SystemChannelID
	}
	if joinLeaveChannel == 0 {
		return
	}

	joinleaveInfo := utils.NewMessageTemplateData(e.Member, guild.Guild)

	contents, err := mustache.RenderRaw(guildSettings.JoinMessage, true, joinleaveInfo)
	if err != nil {
		slog.Error(
			"Failed to render join message template.",
			"err", err,
			"guild_id", guildID,
		)
		return
	}

	_, err = e.Client().Rest.CreateMessage(
		joinLeaveChannel, discord.NewMessageCreateBuilder().SetContent(contents).Build(),
	)
	if err != nil {
		slog.Error(
			"Failed to send join message.",
			"guild_id", guildID,
			"channel_id", joinLeaveChannel,
			"err", err,
		)
	}
}

func OnUserLeave(e *events.GuildMemberLeave) {
	guildID := e.GuildID
	guild, err := e.Client().Rest.GetGuild(guildID, false)
	if err != nil {
		return
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return
	}

	if !guildSettings.LeaveMessageEnabled {
		return
	}

	joinLeaveChannel := guildSettings.JoinLeaveChannel
	if joinLeaveChannel == 0 && guild.SystemChannelID != nil {
		joinLeaveChannel = *guild.SystemChannelID
	}
	if joinLeaveChannel == 0 {
		return
	}

	if pruned, _ := model.IsMemberPruned(guildID, e.User.ID); pruned {
		return
	}

	e.Member.User = e.User
	joinleaveInfo := utils.NewMessageTemplateData(e.Member, guild.Guild)

	contents, err := mustache.RenderRaw(guildSettings.LeaveMessage, true, joinleaveInfo)
	if err != nil {
		slog.Error(
			"Failed to render leave message template.",
			"err", err,
			"guild_id", guildID,
		)
		return
	}

	_, err = e.Client().Rest.CreateMessage(
		joinLeaveChannel, discord.NewMessageCreateBuilder().SetContent(contents).Build(),
	)
	if err != nil {
		slog.Error(
			"Failed to send leave message.",
			"guild_id", guildID,
			"channel_id", joinLeaveChannel,
			"err", err,
		)
	}
}

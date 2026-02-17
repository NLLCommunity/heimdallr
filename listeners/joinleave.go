package listeners

import (
	"log/slog"
	"strings"

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

	if guildSettings.JoinMessageV2 && guildSettings.JoinMessageV2Json != "" {
		emojiMap := make(map[string]discord.Emoji)
		for emoji := range e.Client().Caches.Emojis(guildID) {
			emojiMap[strings.ToLower(emoji.Name)] = emoji
		}

		components, err := utils.BuildV2Message(guildSettings.JoinMessageV2Json, joinleaveInfo, emojiMap)
		if err != nil {
			slog.Error("Failed to build V2 join message.", "err", err, "guild_id", guildID)
			return
		}

		_, err = e.Client().Rest.CreateMessage(joinLeaveChannel, discord.MessageCreate{
			Flags:      discord.MessageFlagIsComponentsV2,
			Components: components,
		})
		if err != nil {
			slog.Error("Failed to send V2 join message.", "guild_id", guildID, "channel_id", joinLeaveChannel, "err", err)
		}
		return
	}

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
		joinLeaveChannel, discord.NewMessageCreate().WithContent(contents),
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

	if guildSettings.LeaveMessageV2 && guildSettings.LeaveMessageV2Json != "" {
		emojiMap := make(map[string]discord.Emoji)
		for emoji := range e.Client().Caches.Emojis(guildID) {
			emojiMap[strings.ToLower(emoji.Name)] = emoji
		}

		components, err := utils.BuildV2Message(guildSettings.LeaveMessageV2Json, joinleaveInfo, emojiMap)
		if err != nil {
			slog.Error("Failed to build V2 leave message.", "err", err, "guild_id", guildID)
			return
		}

		_, err = e.Client().Rest.CreateMessage(joinLeaveChannel, discord.MessageCreate{
			Flags:      discord.MessageFlagIsComponentsV2,
			Components: components,
		})
		if err != nil {
			slog.Error("Failed to send V2 leave message.", "guild_id", guildID, "channel_id", joinLeaveChannel, "err", err)
		}
		return
	}

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
		joinLeaveChannel, discord.NewMessageCreate().WithContent(contents),
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

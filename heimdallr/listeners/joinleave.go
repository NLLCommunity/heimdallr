package listeners

import (
	"log/slog"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

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

	hasV2 := guildSettings.JoinMessageV2 && guildSettings.JoinMessageV2Json != ""

	if hasV2 {
		emojiMap := make(map[string]discord.Emoji)
		for emoji := range e.Client().Caches.Emojis(guildID) {
			emojiMap[strings.ToLower(emoji.Name)] = emoji
		}

		_, err = createV2Message(joinleaveInfo, emojiMap, guildSettings.JoinMessageV2Json, guildID, joinLeaveChannel, e.Client())
	} else {
		_, err = createV1Message(joinleaveInfo, guildSettings.JoinMessage, guildID, joinLeaveChannel, e.Client())
	}

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

	hasV2 := guildSettings.LeaveMessageV2 && guildSettings.LeaveMessageV2Json != ""

	if hasV2 {
		emojiMap := make(map[string]discord.Emoji)
		for emoji := range e.Client().Caches.Emojis(guildID) {
			emojiMap[strings.ToLower(emoji.Name)] = emoji
		}

		_, err = createV2Message(joinleaveInfo, emojiMap, guildSettings.LeaveMessageV2Json, guildID, joinLeaveChannel, e.Client())
	} else {
		_, err = createV1Message(joinleaveInfo, guildSettings.LeaveMessage, guildID, joinLeaveChannel, e.Client())
	}

	if err != nil {
		slog.Error(
			"Failed to send leave message.",
			"guild_id", guildID,
			"channel_id", joinLeaveChannel,
			"err", err,
		)
	}
}

func createV1Message(
	data utils.MessageTemplateData,
	messageTemplate string,
	guildID snowflake.ID,
	channelID snowflake.ID,
	client *bot.Client,
) (m *discord.Message, err error) {

	contents, err := mustache.RenderRaw(messageTemplate, true, data)
	if err != nil {
		slog.Error("Failed to render V1 join message template.", "err", err, "guild_id", guildID)
		return
	}

	m, err = client.Rest.CreateMessage(
		channelID, discord.NewMessageCreate().WithContent(contents),
	)
	if err != nil {
		slog.Error("Failed to send V1 join message.", "guild_id", guildID, "channel_id", channelID, "err", err)
	}
	return
}

func createV2Message(
	data utils.MessageTemplateData,
	emojiMap map[string]discord.Emoji,
	messageJson string,
	guildID snowflake.ID,
	channelID snowflake.ID,
	client *bot.Client,
) (m *discord.Message, err error) {

	components, err := utils.BuildV2Message(messageJson, data, emojiMap)
	if err != nil {
		slog.Error("Failed to build V2 join message.", "err", err, "guild_id", guildID)
		return
	}

	m, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: components,
	})
	if err != nil {
		slog.Error("Failed to send V2 join message.", "guild_id", guildID, "channel_id", channelID, "err", err)
	}
	return
}

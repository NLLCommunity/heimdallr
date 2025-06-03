package listeners

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func OnUserVoiceJoin(e *events.GuildVoiceJoin) {
	settings, err := model.GetGuildSettings(e.VoiceState.GuildID)
	if err != nil || settings.LogChannel == 0 {
		return
	}

	e.Client().Rest().CreateMessage(
		settings.LogChannel,
		discord.NewMessageCreateBuilder().
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetContentf(
				"Voice channel joined: %s joined %s",
				e.Member.Mention(),
				utils.MentionChannelOrDefault(e.VoiceState.ChannelID, "unknown"),
			).Build(),
	)
}

func OnUserVoiceLeave(e *events.GuildVoiceLeave) {
	settings, err := model.GetGuildSettings(e.OldVoiceState.GuildID)
	if err != nil || settings.LogChannel == 0 {
		return
	}

	e.Client().Rest().CreateMessage(
		settings.LogChannel,
		discord.NewMessageCreateBuilder().
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetContentf(
				"Voice channel left: %s left %s",
				e.Member.Mention(),
				utils.MentionChannelOrDefault(e.OldVoiceState.ChannelID, "unknown"),
			).Build(),
	)
}

func OnUserVoiceMove(e *events.GuildVoiceMove) {
	settings, err := model.GetGuildSettings(e.OldVoiceState.GuildID)
	if err != nil || settings.LogChannel == 0 {
		return
	}

	e.Client().Rest().CreateMessage(
		settings.LogChannel,
		discord.NewMessageCreateBuilder().
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetContentf(
				"Voice channel changed: %s changed from %s to %s",
				e.Member.Mention(),
				utils.MentionChannelOrDefault(e.OldVoiceState.ChannelID, "unknown"),
				utils.MentionChannelOrDefault(e.VoiceState.ChannelID, "unknown"),
			).Build(),
	)
}

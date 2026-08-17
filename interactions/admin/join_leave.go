package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func AdminJoinLeaveHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)

	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	data := e.SlashCommandInteractionData()

	resetOption, hasReset := data.OptString("reset")
	if hasReset {
		switch resetOption {
		case "join-enabled":
			settings.JoinMessageEnabled = false
			message += "Join message enabled has been reset.\n"
		case "leave-enabled":
			settings.LeaveMessageEnabled = false
			message += "Leave message enabled has been reset.\n"
		case "channel":
			settings.JoinLeaveChannel = 0
			message += "Join/leave channel has been reset.\n"
		case "all":
			settings.JoinMessageEnabled = false
			settings.LeaveMessageEnabled = false
			settings.JoinLeaveChannel = 0
			message += "All join/leave settings have been reset.\n"
		}
	}

	joinEnabled, hasJoinEnabled := data.OptBool("join-enabled")
	if hasJoinEnabled {
		settings.JoinMessageEnabled = joinEnabled
		message += fmt.Sprintf("Join message enabled set to %s\n", utils.Iif(joinEnabled, "yes", "no"))
	}

	leaveEnabled, hasLeaveEnabled := data.OptBool("leave-enabled")
	if hasLeaveEnabled {
		settings.LeaveMessageEnabled = leaveEnabled
		message += fmt.Sprintf("Leave message enabled set to %s\n", utils.Iif(leaveEnabled, "yes", "no"))
	}

	channel, hasChannel := data.OptChannel("channel")
	if hasChannel {
		settings.JoinLeaveChannel = channel.ID
		message += fmt.Sprintf("Join/leave channel set to <#%d>\n", channel.ID)
	}

	if !utils.Any(hasJoinEnabled, hasLeaveEnabled, hasChannel, hasReset) {
		return e.CreateMessage(interactions.EphemeralMessageContent(joinLeaveInfo(settings)))
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}
	logSettingsCommandUpdate(guild.ID, e.User(), "join_leave", map[string]any{
		"join_message_enabled":  settings.JoinMessageEnabled,
		"leave_message_enabled": settings.LeaveMessageEnabled,
		"join_leave_channel":    settings.JoinLeaveChannel.String(),
	})

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

func joinLeaveInfo(settings *model.GuildSettings) string {
	joinMessageEnabledInfo := "> This determines whether to enable join messages."
	joinMessageEnabled := fmt.Sprintf(
		"**Join message enabled:** %s\n%s",
		utils.Iif(settings.JoinMessageEnabled, "yes", "no"), joinMessageEnabledInfo,
	)

	leaveMessageEnabledInfo := "> This determines whether to enable leave messages."
	leaveMessageEnabled := fmt.Sprintf(
		"**Leave message enabled:** %s\n%s",
		utils.Iif(settings.LeaveMessageEnabled, "yes", "no"), leaveMessageEnabledInfo,
	)

	joinLeaveChannelInfo := "> This is the channel in which join and leave messages are sent."
	joinLeaveChannel := fmt.Sprintf(
		"**Join/leave channel:** %s\n%s",
		utils.MentionChannelOrDefault(&settings.JoinLeaveChannel, "not set"),
		joinLeaveChannelInfo,
	)

	joinLeaveMessageInfo := "The join/leave messages can be viewed by using the `/admin join-message` and `/admin leave-message` commands."

	return fmt.Sprintf(
		"## Join/leave settings\n%s\n\n%s\n\n%s\n\n*%s*",
		joinMessageEnabled, leaveMessageEnabled, joinLeaveChannel, joinLeaveMessageInfo,
	)
}

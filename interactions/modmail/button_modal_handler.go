package modmail

import (
	"fmt"
	"log/slog"
	"strconv"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
)

func ModmailReportModalHandler(e *handler.ModalEvent) error {
	_ = e.DeferCreateMessage(true)

	role := e.Vars["role"]
	channel := e.Vars["channel"]
	maxActiveStr := e.Vars["max-active"]
	slowModeStr := e.Vars["slow-mode"]

	maxActive, err := strconv.Atoi(maxActiveStr)
	if err != nil {
		slog.Warn("Failed to parse 'max active'.", "max_active", maxActiveStr, "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report").
				Build())
		return err
	}

	slowMode, err := strconv.Atoi(slowModeStr)
	if err != nil {
		slog.Error("Failed to parse 'slow mode'.", "slow_mode", slowModeStr, "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report").
				Build())
		return err
	}

	canSubmit, err := isBelowMaxActive(e, maxActive)
	if err != nil {
		slog.Error("Failed to check if user can submit report",
			"max-active", maxActiveStr, "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report.").
				Build())
		return err
	}

	if !canSubmit {
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("You have reached the maximum number of active reports.").
				Build())
		return err
	}

	title := e.Data.Text("title")
	description := e.Data.Text("description")

	thread, err := e.Client().Rest().CreateThread(
		e.Channel().ID(),
		discord.GuildPrivateThreadCreate{
			Name:                title,
			AutoArchiveDuration: 10080,
			Invitable:           utils.Ref(false),
		})
	if err != nil {
		slog.Error("Failed to create thread", "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report.").
				Build())
		return err
	}

	if slowMode > 0 {
		_, err := e.Client().Rest().UpdateChannel(thread.ID(), discord.GuildThreadUpdate{
			RateLimitPerUser: utils.Ref(slowMode),
		})
		if err != nil {
			slog.Error("Failed to update thread rate limit", "err", err, "channel", thread.ID())
		}
	}

	user := e.User()
	avatarURL := getUserAvatarURL(&user)

	embed := discord.NewEmbedBuilder().
		SetTitle(title).
		SetDescription(description).
		SetColor(0x4848FF).
		SetAuthor(user.Username, "", avatarURL).
		Build()

	message, err := e.Client().Rest().CreateMessage(
		thread.ID(),
		discord.MessageCreate{
			Content: fmt.Sprintf("||%s%s",
				utils.Iif(role != "0", fmt.Sprintf("<@&%s>", role), ""),
				user.Mention()),
			Embeds: []discord.Embed{embed},
		})
	if err != nil {
		return err
	}

	if channel != "" && channel != "0" {
		channelSnowflake, err := snowflake.Parse(channel)
		if err == nil {
			_, err = e.Client().Rest().CreateMessage(channelSnowflake,
				discord.NewMessageCreateBuilder().
					SetContentf("### New Modmail thread in <#%d>", e.Channel().ID()).
					AddEmbeds(embed).
					AddActionRow(
						discord.NewLinkButton("Go to thread", message.JumpURL()),
					).Build())

			if err != nil {
				slog.Error("Failed to send message to Modmail notification channel",
					"err", err, "channel", channel)
			}
		}
	}

	_, err = e.CreateFollowupMessage(
		ix.EphemeralMessageContent("Report created!").
			AddActionRow(discord.NewLinkButton("View", message.JumpURL())).
			Build())

	return err
}

func getUserAvatarURL(user *discord.User) string {
	avatarURL := user.AvatarURL()
	if avatarURL != nil {
		return *avatarURL
	}

	return user.DefaultAvatarURL()
}

package modmail

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/interactions/quote"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var ModmailReportMessageCommand = discord.MessageCommandCreate{
	Name:     "Report Message",
	Contexts: []discord.InteractionContextType{discord.InteractionContextTypeGuild},
}

func ModmailReportMessageHandler(e *handler.CommandEvent) error {
	message := e.MessageCommandInteractionData().TargetMessage()

	customID := fmt.Sprintf("/modmail/report-message/%s/%s", message.ChannelID, message.ID)

	modal := discord.NewModalCreateBuilder().
		SetCustomID(customID).
		AddActionRow(
			discord.NewParagraphTextInput("reason", "Report reason").
				WithPlaceholder("The reason for reporting the message."),
		).Build()

	return e.Modal(modal)
}

func ModmailReportMessageModalHandler(e *handler.ModalEvent) error {
	channelIDStr := e.Vars["channelID"]
	messageIDStr := e.Vars["messageID"]

	reason := e.Data.Text("reason")

	_ = e.DeferCreateMessage(true)

	settings, err := model.GetModmailSettings(*e.GuildID())
	if err != nil {
		slog.Warn("Failed to get Modmail settings for guild.", "guild", *e.GuildID())
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Something went wrong when getting guild settings.").
				Build())
		return err
	}

	guildSettings, err := model.GetGuildSettings(*e.GuildID())
	if err != nil {
		slog.Warn("Failed to get Guild settings for guild.", "guild", *e.GuildID())
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Something went wrong when getting guild settings.").
				Build())
		return err
	}

	if settings.ReportThreadsChannel == 0 {
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("This guild has not set up message reports.").
				Build())
		return err
	}

	channelID, err := snowflake.Parse(channelIDStr)
	if err != nil {
		slog.Error("Failed to parse channel ID during message report.", "channel_id", channelIDStr)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to pocess report.").
				Build())
		return err
	}

	messageID, err := snowflake.Parse(messageIDStr)
	if err != nil {
		slog.Error("Failed to parse message ID during message report.", "message_id", messageIDStr)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to pocess report.").
				Build())
		return err
	}

	message, err := e.Client().Rest().GetMessage(channelID, messageID)
	if err != nil {
		slog.Error("Failed to get message for report", "channel", channelID, "message", messageID)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to retrieve reported message.").
				Build())
		return err
	}

	messageEmbed := quote.CreateMessageQuoteEmbed(e.Client(), message, true)
	messageEmbed.Color = 0xFF0000

	data := reportData{
		event:           e,
		modmailSettings: settings,
		guildSettings:   guildSettings,
		message:         message,
		messageEmbed:    messageEmbed,
		reason:          reason,
	}

	err = createReportThreadAndMessage(&data)
	if err != nil {
		slog.Error("Failed to create report message")
		_, err := e.CreateFollowupMessage(ix.EphemeralMessageContent("Failed to create report.").
			Build())
		return err
	}

	_, _ = e.CreateFollowupMessage(ix.EphemeralMessageContent("Report created!").
		AddActionRow(discord.NewLinkButton("Go to thread", data.reportMessage.JumpURL())).
		Build())

	err = notifyReportMessage(&data)
	if err != nil {
		slog.Error("Failed to notify new report message")
	}

	return err
}

type reportData struct {
	event           *handler.ModalEvent
	modmailSettings *model.ModmailSettings
	guildSettings   *model.GuildSettings
	message         *discord.Message
	messageEmbed    discord.Embed
	reason          string

	reportMessage *discord.Message
}

func createReportThreadAndMessage(data *reportData) error {
	user := data.event.User()
	title := fmt.Sprintf("%s's message report", user.EffectiveName())
	thread, err := data.event.Client().Rest().CreateThread(
		data.modmailSettings.ReportThreadsChannel,
		discord.GuildPrivateThreadCreate{
			Name:                title,
			AutoArchiveDuration: 10080,
			Invitable:           utils.Ref(false),
		})
	if err != nil {
		slog.Error("Failed to create thread", "err", err)
		_, err := data.event.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report.").
				Build())
		return err
	}

	reasonEmbed := discord.NewEmbedBuilder().
		SetAuthor(user.Username, "", getUserAvatarURL(&user)).
		SetTitle("Message Report").
		SetDescription(data.reason).
		Build()

	messageText := user.Mention()
	if data.modmailSettings.ReportPingRole != 0 {
		messageText += fmt.Sprintf(" <@&%s>", data.modmailSettings.ReportPingRole)
	}

	reportMessage, err := data.event.Client().Rest().CreateMessage(thread.ID(),
		discord.NewMessageCreateBuilder().
			SetContent(messageText).
			AddEmbeds(reasonEmbed, data.messageEmbed).
			AddActionRow(discord.NewLinkButton("Jump to message", data.message.JumpURL())).
			Build())

	if err != nil {
		slog.Warn("Failed to create report message")
		_, _ = data.event.CreateFollowupMessage(ix.EphemeralMessageContent("Failed to create report.").Build())
		return err
	}

	data.reportMessage = reportMessage

	return nil
}

func notifyReportMessage(data *reportData) error {
	var channelID snowflake.ID

	guild, gErr := data.event.Client().Rest().GetGuild(*data.event.GuildID(), false)

	if data.modmailSettings.ReportNotificationChannel != 0 {
		channelID = data.modmailSettings.ReportNotificationChannel
	} else if data.guildSettings.ModeratorChannel != 0 {
		channelID = data.guildSettings.ModeratorChannel
	} else if gErr == nil && guild.SystemChannelID != nil {
		channelID = *guild.SystemChannelID
	}

	if channelID == 0 {
		return nil // no channel to notify
	}

	message := discord.NewMessageCreateBuilder().
		SetContentf("New message report from %s", data.event.User().Mention()).
		SetEmbeds(data.reportMessage.Embeds...).
		AddActionRow(
			discord.NewLinkButton("Go to report", data.reportMessage.JumpURL()),
			discord.NewLinkButton("Go to message", data.message.JumpURL())).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build()

	_, err := data.event.Client().Rest().CreateMessage(channelID, message)
	if err != nil {
		slog.Warn("Failed to notify message report.")
		return err
	}

	return nil
}

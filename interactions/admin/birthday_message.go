package admin

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func AdminBirthdayMessageHandler(event *handler.CommandEvent) error {
	utils.LogInteraction("admin", event)
	guild, ok := event.Guild()
	if !ok {
		return interactions.ErrEventNoGuildID
	}
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}
	reset, hasReset := event.SlashCommandInteractionData().OptString("reset")
	if hasReset && reset == "reset" {
		settings.BirthdayMessage = model.DefaultBirthdayMessage
		settings.BirthdayMessageV2 = false
		settings.BirthdayMessageV2Json = ""
		if err := model.UpdateGuildSettingsColumns(settings,
			"BirthdayMessage", "BirthdayMessageV2", "BirthdayMessageV2Json",
		); err != nil {
			return err
		}
		logSettingsCommandUpdate(guild.ID, event.User(), "birthday", map[string]any{"mode": "plain"})
		return event.CreateMessage(interactions.EphemeralMessageContent("Birthday message has been reset."))
	}
	if settings.BirthdayMessageV2 {
		return event.CreateMessage(interactions.EphemeralMessageContent(
			"The birthday message is using **Components V2** mode, which can only be edited from the web dashboard. Use `reset` to return to plain text mode.",
		))
	}
	return event.CreateMessage(
		interactions.EphemeralMessageContent("").
			WithEmbeds(
				discord.NewEmbed().WithTitle("Birthday message").WithDescription(settings.BirthdayMessage),
				discord.NewEmbed().WithTitle("Placeholder values").WithDescription(utils.BirthdayMessageTemplateInfo()),
			).
			AddActionRow(discord.NewPrimaryButton("Edit message", adminBirthdayMessageButtonRoute.StaticCustomID())),
	)
}

func AdminBirthdayMessageButtonHandler(event *handler.ComponentEvent) error {
	utils.LogInteraction("admin", event)
	guild, ok := event.Guild()
	if !ok {
		return interactions.ErrEventNoGuildID
	}
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}
	if settings.BirthdayMessageV2 {
		return event.CreateMessage(interactions.EphemeralMessageContent(
			"This message uses Components V2 and can only be edited from the web dashboard.",
		))
	}
	return event.Modal(messageModal(
		adminBirthdayMessageModalRoute.StaticCustomID(),
		"Birthday message",
		settings.BirthdayMessage,
	))
}

func AdminBirthdayMessageModalHandler(event *handler.ModalEvent) error {
	utils.LogInteraction("admin", event)
	guild, ok := event.Guild()
	if !ok {
		return interactions.ErrEventNoGuildID
	}
	message := event.Data.Text("message")
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}
	settings.BirthdayMessage = message
	settings.BirthdayMessageV2 = false
	settings.BirthdayMessageV2Json = ""
	if err := model.ValidateBirthdaySettings(settings); err != nil {
		return event.CreateMessage(interactions.EphemeralMessageContent(err.Error()))
	}
	if err := model.UpdateGuildSettingsColumns(settings,
		"BirthdayMessage", "BirthdayMessageV2", "BirthdayMessageV2Json",
	); err != nil {
		return err
	}
	logSettingsCommandUpdate(guild.ID, event.User(), "birthday", map[string]any{"mode": "plain"})
	return event.CreateMessage(interactions.EphemeralMessageContent("Birthday message updated."))
}

package admin

import (
	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var gatekeepMessageSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "gatekeep-message",
	Description: "Set the message to send to approved users",
}

func AdminGatekeepMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin gatekeep message", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Gatekeep approved message").
		SetDescription(settings.GatekeepApprovedMessage).
		Build()

	templateInfoEmbed := discord.NewEmbedBuilder().
		SetTitle("Placeholder values").
		SetDescription(utils.MessageTemplateInfo).
		Build()

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/gatekeep-message/button")).
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
}

func AdminGatekeepMessageButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin gatekeep message button", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	return e.Modal(
		messageModal(
			"/admin/gatekeep-message/modal",
			"Gatekeep approved message",
			settings.GatekeepApprovedMessage,
		),
	)
}

func AdminGatekeepMessageModalHandler(e *handler.ModalEvent) error {
	utils.LogInteraction("admin gatekeep message modal", e)

	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	message := e.Data.Text("message")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	_, err = mustache.RenderRaw(message, true, utils.MessageTemplateData{})
	if err != nil {
		return interactions.MessageEphWithContentf(
			e, "The message contains data that is invalid; this may be caused by invalid placeholders.",
		)
	}

	settings.GatekeepApprovedMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return interactions.MessageEphWithContentf(e, "Gatekeep approved message updated.")
}

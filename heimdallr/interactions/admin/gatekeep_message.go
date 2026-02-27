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
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset the message to its default value",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Reset", Value: "reset"},
			},
		},
	},
}

func AdminGatekeepMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	data := e.SlashCommandInteractionData()
	resetOption, hasReset := data.OptString("reset")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if hasReset && resetOption == "reset" {
		settings.GatekeepApprovedMessage = "Welcome to the server, {{user}}!"
		settings.GatekeepApprovedMessageV2 = false
		settings.GatekeepApprovedMessageV2Json = ""
		err = model.SetGuildSettings(settings)
		if err != nil {
			return err
		}
		return e.CreateMessage(interactions.EphemeralMessageContent("Gatekeep approved message has been reset."))
	}

	if settings.GatekeepApprovedMessageV2 {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"The gatekeep approved message is currently using **Components V2** mode, which can only be edited from the web dashboard.\n\nUse the `reset` option to switch back to plain text mode.",
			),
		)
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
		interactions.EphemeralMessageContent("").
			WithEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/gatekeep-message/button")),
	)
}

func AdminGatekeepMessageButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if settings.GatekeepApprovedMessageV2 {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"This message uses Components V2 and can only be edited from the web dashboard.",
			),
		)
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
	utils.LogInteraction("admin", e)

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
		return e.CreateMessage(
			interactions.EphemeralMessageContent("The message contains data that is invalid; this may be caused by invalid placeholders."),
		)
	}

	settings.GatekeepApprovedMessage = message
	settings.GatekeepApprovedMessageV2 = false
	settings.GatekeepApprovedMessageV2Json = ""

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent("Gatekeep approved message updated."))
}

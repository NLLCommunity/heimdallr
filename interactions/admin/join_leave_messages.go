package admin

import (
	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var joinMessageSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "join-message",
	Description: "Set the message to send when a user joins",
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

var leaveMessageSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "leave-message",
	Description: "Set the message to send when a user leaves",
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

func AdminJoinMessageHandler(e *handler.CommandEvent) error {
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
		settings.JoinMessage = "Welcome to the server, {{user}}!"
		settings.JoinMessageV2 = false
		settings.JoinMessageV2Json = ""
		err = model.SetGuildSettings(settings)
		if err != nil {
			return err
		}
		return e.CreateMessage(interactions.EphemeralMessageContent("Join message has been reset."))
	}

	if settings.JoinMessageV2 {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"The join message is currently using **Components V2** mode, which can only be edited from the web dashboard.\n\nUse the `reset` option to switch back to plain text mode.",
			),
		)
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Join message").
		SetDescription(settings.JoinMessage).
		Build()

	templateInfoEmbed := discord.NewEmbedBuilder().
		SetTitle("Placeholder values").
		SetDescription(utils.MessageTemplateInfo).
		Build()

	return e.CreateMessage(
		interactions.EphemeralMessageContent("").
			WithEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/join-message/button")),
	)
}

func AdminJoinMessageButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if settings.JoinMessageV2 {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"This message uses Components V2 and can only be edited from the web dashboard.",
			),
		)
	}

	return e.Modal(
		messageModal(
			"/admin/join-message/modal",
			"Join message",
			settings.JoinMessage,
		),
	)
}

func AdminJoinMessageModalHandler(e *handler.ModalEvent) error {
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
			interactions.EphemeralMessageContent(
				"The message contains data that is invalid; this may be caused by invalid placeholders.",
			),
		)
	}

	settings.JoinMessage = message
	settings.JoinMessageV2 = false
	settings.JoinMessageV2Json = ""

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent("Join message updated."))
}

func AdminLeaveMessageHandler(e *handler.CommandEvent) error {
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
		settings.LeaveMessage = "{{user}} has left the server."
		settings.LeaveMessageV2 = false
		settings.LeaveMessageV2Json = ""
		err = model.SetGuildSettings(settings)
		if err != nil {
			return err
		}
		return e.CreateMessage(interactions.EphemeralMessageContent("Leave message has been reset."))
	}

	if settings.LeaveMessageV2 {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"The leave message is currently using **Components V2** mode, which can only be edited from the web dashboard.\n\nUse the `reset` option to switch back to plain text mode.",
			),
		)
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Leave message").
		SetDescription(settings.LeaveMessage).
		Build()

	templateInfoEmbed := discord.NewEmbedBuilder().
		SetTitle("Placeholder values").
		SetDescription(utils.MessageTemplateInfo).
		Build()

	return e.CreateMessage(
		interactions.EphemeralMessageContent("").
			WithEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/leave-message/button")),
	)
}

func AdminLeaveMessageButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if settings.LeaveMessageV2 {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"This message uses Components V2 and can only be edited from the web dashboard.",
			),
		)
	}

	return e.Modal(
		messageModal(
			"/admin/leave-message/modal",
			"Leave message",
			settings.LeaveMessage,
		),
	)
}

func AdminLeaveMessageModalHandler(e *handler.ModalEvent) error {
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
			interactions.EphemeralMessageContent(
				"The message contains data that is invalid; this may be caused by invalid placeholders.",
			),
		)
	}

	settings.LeaveMessage = message
	settings.LeaveMessageV2 = false
	settings.LeaveMessageV2Json = ""

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent("Leave message updated."))
}

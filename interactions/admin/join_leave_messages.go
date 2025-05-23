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
		// Reset the message to default
		settings.JoinMessage = "Welcome to the server, {{user}}!"
		err = model.SetGuildSettings(settings)
		if err != nil {
			return err
		}
		return e.CreateMessage(interactions.EphemeralMessageContent("Join message has been reset.").Build())
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
			SetEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/join-message/button")).
			Build(),
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
			).
				Build(),
		)
	}

	settings.JoinMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent("Join message updated.").Build())
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
		// Reset the message to default
		settings.LeaveMessage = "{{user}} has left the server."
		err = model.SetGuildSettings(settings)
		if err != nil {
			return err
		}
		return e.CreateMessage(interactions.EphemeralMessageContent("Leave message has been reset.").Build())
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
			SetEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/leave-message/button")).
			Build(),
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
			).
				Build(),
		)
	}

	settings.LeaveMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent("Leave message updated.").Build())
}

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
}

var leaveMessageSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "leave-message",
	Description: "Set the message to send when a user leaves",
}

func AdminJoinMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin join-message", e)
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
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
		discord.NewMessageCreateBuilder().
			SetEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/join-message/button")).
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetEphemeral(true).
			Build(),
	)
}

func AdminJoinMessageButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin join-message button", e)

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
	utils.LogInteraction("admin join-message modal", e)

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
			discord.NewMessageCreateBuilder().
				SetContentf("The message contains data that is invalid; this may be caused by invalid placeholders.").
				SetEphemeral(true).
				Build(),
		)
	}

	settings.JoinMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetContent("Join message updated.").
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
}

func AdminLeaveMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin leave-message", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
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
		discord.NewMessageCreateBuilder().
			SetEmbeds(embed, templateInfoEmbed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/leave-message/button")).
			SetAllowedMentions(&discord.AllowedMentions{}).
			SetEphemeral(true).
			Build(),
	)
}

func AdminLeaveMessageButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin leave-message button", e)

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
	utils.LogInteraction("admin leave-message modal", e)

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
			discord.NewMessageCreateBuilder().
				SetContentf("The message contains data that is invalid; this may be caused by invalid placeholders.").
				SetEphemeral(true).
				Build(),
		)
	}

	settings.LeaveMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetContent("Leave message updated.").
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
}

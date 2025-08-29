package admin

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var banFooterSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "ban-footer",
	Description: "View or set the ban footer, shown at the end of DM sent when user is banned.",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionBool{
			Name:        "always-send",
			Description: "Whether to always send the footer, even if there is no ban message",
		},
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

func AdminBanFooterHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)
	guild, isGuild := e.Guild()
	if !isGuild {
		return ix.ErrEventNoGuildID
	}

	data := e.SlashCommandInteractionData()
	resetOption, hasReset := data.OptString("reset")
	alwaysSend, hasAlwaysSend := data.OptBool("always-send")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if hasAlwaysSend {
		settings.AlwaysSendBanFooter = alwaysSend
		err = model.SetGuildSettings(settings)
		if err != nil {
			return e.CreateMessage(ix.EphemeralMessageContent("Failed to save settings.").Build())
		}
		return e.CreateMessage(ix.EphemeralMessageContent("Settings saved.").Build())
	}

	if hasReset && resetOption == "reset" {
		settings.BanFooter = ""
		settings.AlwaysSendBanFooter = false
		err = model.SetGuildSettings(settings)
		if err != nil {
			return err
		}
		return e.CreateMessage(ix.EphemeralMessageContent("Ban footer has been reset.").Build())
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Ban footer").
		SetDescription(utils.Iif(settings.BanFooter == "", "*no footer set*", settings.BanFooter)).
		Build()

	return e.CreateMessage(
		ix.EphemeralMessageContentf("**Always send ban DM:** %s",
			utils.Iif(settings.AlwaysSendBanFooter, "yes", "no")).
			SetEmbeds(embed).
			AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/ban-footer/button")).
			Build())
}

func AdminBanFooterButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return ix.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	return e.Modal(
		messageModal(
			"/admin/ban-footer/modal",
			"Ban footer",
			settings.BanFooter,
		),
	)
}

func AdminBanFooterModalHandler(e *handler.ModalEvent) error {
	utils.LogInteraction("admin", e)
	guild, isGuild := e.Guild()
	if !isGuild {
		return ix.ErrEventNoGuildID
	}

	message := e.Data.Text("message")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	settings.BanFooter = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to update ban footer").Build())
	}

	return e.CreateMessage(ix.EphemeralMessageContent("Ban footer updated.").Build())
}

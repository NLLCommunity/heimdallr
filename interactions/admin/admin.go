package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Route(
		"/admin", func(r handler.Router) {
			r.Component("/show-all-button", AdminShowAllButtonHandler)
			r.Command("/info", AdminInfoHandler)
			r.Command("/mod-channel", AdminModChannelHandler)
			r.Command("/infractions", AdminInfractionsHandler)
			r.Command("/gatekeep", AdminGatekeepHandler)
			r.Command("/gatekeep-message", AdminGatekeepMessageHandler)
			r.Component("/gatekeep-message/button", AdminGatekeepMessageButtonHandler)
			r.Modal("/gatekeep-message/modal", AdminGatekeepMessageModalHandler)
			r.Command("/join-leave", AdminJoinLeaveHandler)

			r.Command("/join-message", AdminJoinMessageHandler)
			r.Component("/join-message/button", AdminJoinMessageButtonHandler)
			r.Modal("/join-message/modal", AdminJoinMessageModalHandler)

			r.Command("/leave-message", AdminLeaveMessageHandler)
			r.Component("/leave-message/button", AdminLeaveMessageButtonHandler)
			r.Modal("/leave-message/modal", AdminLeaveMessageModalHandler)

			r.Command("/anti-spam", AdminAntiSpamHandler)

			r.Command("/ban-footer", AdminBanFooterHandler)
			r.Component("/ban-footer/button", AdminBanFooterButtonHandler)
			r.Modal("/ban-footer/modal", AdminBanFooterModalHandler)
		},
	)

	return []discord.ApplicationCommandCreate{AdminCommand}
}

var AdminCommand = discord.SlashCommandCreate{
	Name:                     "admin",
	Description:              "admin commands",
	DefaultMemberPermissions: omit.NewPtr(discord.PermissionAdministrator),
	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "info",
			Description: "Show information about server configuration",
		},

		modChannelSubcommand,
		infractionsSubCommand,
		gatekeepSubcommand,
		gatekeepMessageSubcommand,
		joinLeaveSubcommand,
		joinMessageSubcommand,
		leaveMessageSubcommand,
		antiSpamSubcommand,
		banFooterSubcommand,
	},
}

func AdminInfoHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)

	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	modChannel := modChannelInfo(settings)
	infractionSettings := infractionInfo(settings)
	gatekeepSettings := gatekeepInfo(settings)
	joinLeaveSettings := joinLeaveInfo(settings)
	antiSpamSettings := antiSpamInfo(settings)

	message := fmt.Sprintf(
		"# Server settings\n%s\n\n%s\n\n%s\n\n%s\n\n%s",
		modChannel, infractionSettings, gatekeepSettings, joinLeaveSettings, antiSpamSettings,
	)

	return e.CreateMessage(
		interactions.EphemeralMessageContent(message).
			AddActionRow(discord.NewPrimaryButton("Display for everyone", "/admin/show-all-button")).
			Build(),
	)
}

func AdminShowAllButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin", e)

	return e.CreateMessage(
		interactions.EphemeralMessageContent(e.Message.Content).
			SetEmbeds(e.Message.Embeds...).
			Build(),
	)
}

func messageModal(customID, title, contents string) discord.ModalCreate {
	return discord.NewModalCreateBuilder().
		SetCustomID(customID).
		SetTitle(title).
		AddLabel(title, discord.NewParagraphTextInput("message").WithValue(contents)).
		Build()
}

package admin

import (
	"strings"

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

			r.Command("/posts", AdminPostsHandler)
			r.Command("/audit-log", AdminAuditLogHandler)
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
		postsSubcommand,
		auditLogSubcommand,
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

	// Each *Info helper returns a self-contained Markdown chunk. When
	// concatenated as a single message body, the assembled result blew
	// past Discord's 2000-char content limit and made /admin info
	// unusable. Render each section as its own embed instead: embed
	// bodies have a 4096-char description cap and a 6000-char total
	// across all embeds in one message, which gives the seven sections
	// comfortable headroom.
	//
	// Titles are passed explicitly here rather than parsed out of the
	// helper output — two helpers (mod_channel, infractions) don't lead
	// with a "## Title" line, and threading a title through the existing
	// Markdown returns would also change the standalone subcommand
	// views, which we're deliberately leaving alone.
	embeds := []discord.Embed{
		sectionEmbed("Moderator channel", modChannelInfo(settings)),
		sectionEmbed("Infractions", infractionInfo(settings)),
		sectionEmbed("Gatekeep", gatekeepInfo(settings)),
		sectionEmbed("Join/Leave", joinLeaveInfo(settings)),
		sectionEmbed("Anti-spam", antiSpamInfo(settings)),
		sectionEmbed("Posts", postsInfo(settings)),
		sectionEmbed("Audit log", auditLogInfo(settings)),
	}

	return e.CreateMessage(
		discord.NewMessageCreate().
			WithEphemeral(true).
			WithAllowedMentions(&discord.AllowedMentions{}).
			WithEmbeds(embeds...).
			AddActionRow(discord.NewPrimaryButton("Display for everyone", "/admin/show-all-button")),
	)
}

// sectionEmbed converts a single *Info helper's Markdown output into an
// embed with the caller-supplied title. If the body itself begins with a
// "## Section heading" line (most helpers do), that line is stripped so
// the heading isn't rendered twice — once in the embed title bar and
// once at the top of the description.
func sectionEmbed(title, rendered string) discord.Embed {
	body := strings.TrimSpace(rendered)
	if rest, ok := strings.CutPrefix(body, "## "); ok {
		if idx := strings.Index(rest, "\n"); idx >= 0 {
			body = strings.TrimSpace(rest[idx+1:])
		} else {
			body = ""
		}
	}
	return discord.Embed{Title: title, Description: body}
}

func AdminShowAllButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("admin", e)

	return e.CreateMessage(
		interactions.EphemeralMessageContent(e.Message.Content).
			WithEmbeds(e.Message.Embeds...),
	)
}

func messageModal(customID, title, contents string) discord.ModalCreate {
	return discord.NewModalCreate(customID, title, nil).
		AddLabel(title, discord.NewParagraphTextInput("message").WithValue(contents))
}

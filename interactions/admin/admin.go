package admin

import (
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

var adminShowAllRoute = rave.Component("/admin/show-all-button").Handle(AdminShowAllButtonHandler)
var adminGatekeepMessageButtonRoute = rave.Component("/admin/gatekeep-message/button").Handle(AdminGatekeepMessageButtonHandler)
var adminGatekeepMessageModalRoute = rave.Modal("/admin/gatekeep-message/modal").Handle(AdminGatekeepMessageModalHandler)
var adminJoinMessageButtonRoute = rave.Component("/admin/join-message/button").Handle(AdminJoinMessageButtonHandler)
var adminJoinMessageModalRoute = rave.Modal("/admin/join-message/modal").Handle(AdminJoinMessageModalHandler)
var adminLeaveMessageButtonRoute = rave.Component("/admin/leave-message/button").Handle(AdminLeaveMessageButtonHandler)
var adminLeaveMessageModalRoute = rave.Modal("/admin/leave-message/modal").Handle(AdminLeaveMessageModalHandler)
var adminBanFooterButtonRoute = rave.Component("/admin/ban-footer/button").Handle(AdminBanFooterButtonHandler)
var adminBanFooterModalRoute = rave.Modal("/admin/ban-footer/modal").Handle(AdminBanFooterModalHandler)

func Register(r handler.Router) []discord.ApplicationCommandCreate {
	adminShowAllRoute.Register(r)
	adminGatekeepMessageButtonRoute.Register(r)
	adminGatekeepMessageModalRoute.Register(r)
	adminJoinMessageButtonRoute.Register(r)
	adminJoinMessageModalRoute.Register(r)
	adminLeaveMessageButtonRoute.Register(r)
	adminLeaveMessageModalRoute.Register(r)
	adminBanFooterButtonRoute.Register(r)
	adminBanFooterModalRoute.Register(r)

	slash := Admin.Register(r)

	return []discord.ApplicationCommandCreate{slash}
}

var Admin = rave.Slash("admin", "admin commands").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionAdministrator).
	AddOptions(
		rave.SubCommand("info", "Show information about server configuration").
			Handle(AdminInfoHandler),

		rave.SubCommand("mod-channel", "Configure the moderator channel").
			AddOptions(
				rave.OptionChannel("channel", "The channel to set as the moderator channel").
					AddChannelTypes(discord.ChannelTypeGuildText),
				rave.OptionString("reset", "Reset the moderator channel").
					AddChoice("Reset", "reset"),
			).Handle(AdminModChannelHandler),

		rave.SubCommand("infractions", "View or set infraction-related settings").
			AddOptions(
				rave.OptionFloat("half-life", "The half-life of infractions in days (0 = no half-life)").
					WithMinValue(0.0).
					WithMaxValue(365.0),
				rave.OptionBool("notify-warned-user-join", "Whether to notify moderator channel when warned user (re)joins the server"),
				rave.OptionFloat("notify-threshold", "The minimum severity of infractions to notify on (0 = always)").
					WithMinValue(0.0).
					WithMaxValue(100.0),
				rave.OptionString("reset", "Reset a setting to its default value").
					AddChoice("Half-life", "half-life").
					AddChoice("Notify on warned user join", "notify-warned-user-join").
					AddChoice("Notify threshold", "notify-threshold").
					AddChoice("All", "all"),
			).Handle(AdminInfractionsHandler),

		rave.SubCommand("gatekeep", "View or set gatekeep-related settings").
			AddOptions(
				rave.OptionBool("enabled", "Whether to enable the gatekeep system"),
				rave.OptionRole("pending-role", "The role to give to users pending approval"),
				rave.OptionRole("approved-role", "The role to give to approved users"),
				rave.OptionBool("use-pending-role", "Whether to give the pending role to users when they join"),
				rave.OptionString("reset", "Reset a setting to its default value").
					AddChoice("Enabled", "enabled").
					AddChoice("Pending role", "pending-role").
					AddChoice("Approved role", "approved-role").
					AddChoice("Use pending role", "use-pending-role").
					AddChoice("All", "all"),
			).Handle(AdminGatekeepHandler),

		rave.SubCommand("gatekeep-message", "View or set the gatekeep message").
			AddOptions(
				rave.OptionString("message", "The message to send to users when they join and are pending approval"),
				rave.OptionString("reset", "Reset the gatekeep message to its default value").
					AddChoice("Reset", "reset"),
			).Handle(AdminGatekeepMessageHandler),

		rave.SubCommand("join-leave", "View or set join/leave-related settings").
			AddOptions(
				rave.OptionBool("join-enabled", "Whether to enable join messages"),
				rave.OptionBool("leave-enabled", "Whether to enable leave messages"),
				rave.OptionChannel("channel", "The channel to send join and leave messages").
					AddChannelTypes(discord.ChannelTypeGuildText),
				rave.OptionString("reset", "Reset a setting to its default value").
					AddChoice("Join enabled", "join-enabled").
					AddChoice("Leave enabled", "leave-enabled").
					AddChoice("Channel", "channel").
					AddChoice("All", "all"),
			).Handle(AdminJoinLeaveHandler),

		rave.SubCommand("join-message", "View or set the join message").
			AddOptions(
				rave.OptionString("reset", "Reset the message to its default value.").
					AddChoice("Reset", "reset"),
			).Handle(AdminJoinMessageHandler),
		rave.SubCommand("leave-message", "View or set the leave message").
			AddOptions(
				rave.OptionString("reset", "Reset the message to its default value.").
					AddChoice("Reset", "reset"),
			).Handle(AdminLeaveMessageHandler),

		rave.SubCommand("anti-spam", "View or set anti-spam settings").
			AddOptions(
				rave.OptionBool("enabled", "Whether to enable the anti-spam system"),
				rave.OptionInt("count", "The number of messages allowed before Heimdallr takes action (within the cooldown period)").
					WithMinValue(1).
					WithMaxValue(15),
				rave.OptionInt("cooldown", "The time in seconds to wait before resetting the message count").
					WithMinValue(1).
					WithMaxValue(60),
				rave.OptionInt("timeout", "The time in minutes to timeout a user who has exceeded the message count").
					WithMinValue(1).
					WithMaxValue(10080), // 7 days
				rave.OptionString("reset", "Reset a setting to its default value").
					AddChoice("Enabled", "enabled").
					AddChoice("Count", "count").
					AddChoice("Cooldown", "cooldown").
					AddChoice("Timeout", "timeout").
					AddChoice("All", "all"),
			).Handle(AdminAntiSpamHandler),

		rave.SubCommand("ban-footer", "View or set the ban footer message").
			AddOptions(
				rave.OptionBool("always-send", "Whether to always send the footer, even if there is no ban message."),
				rave.OptionString("reset", "Reset the message to its default value.").
					AddChoice("Reset", "reset"),
			).Handle(AdminBanFooterHandler),

		rave.SubCommand("posts", "View or set posts-related settings").
			AddOptions(
				rave.OptionRole("mod-role", "Role allowed to manage posts in the dashboard (admins always have access)."),
				rave.OptionString("reset", "Reset a setting to its default value").
					AddChoice("Mod role", "mod-role").
					AddChoice("All", "all"),
			).Handle(AdminPostsHandler),

		rave.SubCommand("audit-log", "View or set audit log settings").
			AddOptions(
				rave.OptionBool("enabled", "Whether to record audit log events for this guild."),
				rave.OptionInt("message-retention", "Override message-event retention in days."),
				rave.OptionInt("member-retention", "Override member-event retention in days."),
				rave.OptionInt("guild-retention", "Override guild-event retention in days."),
				rave.OptionString("reset", "Reset a setting to use the bot-operator default.").
					AddChoice("Enabled", "enabled").
					AddChoice("Message retention", "message-retention").
					AddChoice("Member retention", "member-retention").
					AddChoice("Guild retention", "guild-retention").
					AddChoice("All", "all"),
			).Handle(AdminAuditLogHandler),
	)

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
			AddActionRow(discord.NewPrimaryButton("Display for everyone", adminShowAllRoute.StaticCustomID())),
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
		discord.NewMessageCreate().
			WithContent(e.Message.Content).
			WithEmbeds(e.Message.Embeds...).
			WithAllowedMentions(&discord.AllowedMentions{}),
	)
}

func messageModal(customID, title, contents string) discord.ModalCreate {
	return discord.NewModalCreate(customID, title, nil).
		AddLabel(title, discord.NewParagraphTextInput("message").WithValue(contents))
}

package modmail

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/modmail-admin/create-button", ModmailAdminCreateButtonHandler)
	r.Command("/modmail-admin/settings", ModmailSettingsHandler)
	r.Component("/modmail/report-button/{role}/{channel}/{max-active}/{slow-mode}", ModmailReportButtonHandler)
	r.Modal("/modmail/report-modal/{role}/{channel}/{max-active}/{slow-mode}", ModmailReportModalHandler)
	r.Command("/Report Message", ModmailReportMessageHandler)
	r.Modal("/modmail/report-message/{channelID}/{messageID}", ModmailReportMessageModalHandler)

	return []discord.ApplicationCommandCreate{ModmailAdminCommand, ModmailReportMessageCommand}
}

var ModmailAdminCommand = discord.SlashCommandCreate{
	Name:                     "modmail-admin",
	Description:              "Commands for receiving and sending Modmail.",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Contexts: []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	},
	Options: []discord.ApplicationCommandOption{
		createSubcommand,
		settingsSubcommand,
	},
}

var settingsSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "settings",
	Description: "Modmail settings",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionChannel{
			Name:         "report-channel",
			Description:  "Channel that reports will go into. (Does not apply to report buttons)",
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
		},
		discord.ApplicationCommandOptionRole{
			Name:        "report-ping-role",
			Description: "The role that will be pinged when a report is made",
		},
		discord.ApplicationCommandOptionChannel{
			Name:         "notification-channel",
			Description:  "Channel that report notifications will be posted to",
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset a setting to its default value.",
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "report-channel", Value: "report-channel"},
				{Name: "notification-channel", Value: "notification-channel"},
				{Name: "report-ping-role", Value: "report-ping-role"},
				{Name: "all", Value: "all"},
			},
		},
	},
}

func ModmailSettingsHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()

	reportChannel, reportChannelOK := data.OptChannel("report-channel")
	pingRole, pingRoleOK := data.OptRole("report-ping-role")
	notificationChannel, notificationChannelOK := data.OptChannel("notification-channel")
	resetOption, resetOptionOK := data.OptString("reset")

	settings, err := model.GetModmailSettings(*e.GuildID())
	if err != nil {
		slog.Warn("Failed to load Modmail settings", "guild", *e.GuildID())
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to load settings.").Build())
	}

	if !reportChannelOK && !pingRoleOK && !notificationChannelOK && !resetOptionOK {
		return e.CreateMessage(
			ix.EphemeralMessageContentf(
				"## Modmail Settings\n"+
					"**Report Channel:** %s\n"+
					"> Channel report threads will be created in.\n\n"+
					"**Notification Channel:** %s\n"+
					"> Channel that notifications about new report threads will be sent to.\n\n"+
					"**Ping Role:** %s\n"+
					"> Role that will be pinged when a new thread is created.",

				utils.Iif(
					settings.ReportThreadsChannel != 0,
					fmt.Sprintf("<#%s>", settings.ReportThreadsChannel),
					"not set",
				),
				utils.Iif(
					settings.ReportNotificationChannel != 0,
					fmt.Sprintf("<#%s>", settings.ReportNotificationChannel),
					"not set",
				),
				utils.Iif(
					settings.ReportPingRole != 0,
					fmt.Sprintf("<@&%s>", settings.ReportPingRole),
					"not set",
				),
			).
				Build(),
		)
	}

	message := ""

	if resetOptionOK {
		switch resetOption {
		case "report-channel":
			settings.ReportThreadsChannel = 0
			message += "Report Channel has been reset.\n"
		case "notification-channel":
			settings.ReportNotificationChannel = 0
			message += "Notification Channel has been reset.\n"
		case "report-ping-role":
			settings.ReportPingRole = 0
			message += "Ping Role has been reset.\n"
		case "all":
			settings.ReportThreadsChannel = 0
			settings.ReportNotificationChannel = 0
			settings.ReportPingRole = 0
			message += "All settings have been reset.\n"
		}
	}

	if reportChannelOK {
		settings.ReportThreadsChannel = reportChannel.ID
		message += fmt.Sprintf("Report Channel set to <#%s>\n", reportChannel.ID)
	}
	if pingRoleOK {
		settings.ReportPingRole = pingRole.ID
		message += fmt.Sprintf("Ping Role set to %s\n", pingRole.Mention())
	}
	if notificationChannelOK {
		settings.ReportNotificationChannel = notificationChannel.ID
		message += fmt.Sprintf("Notification Channel set to <#%s>\n", notificationChannel.ID)
	}

	err = model.SetModmailSettings(settings)
	if err != nil {
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to save settings.").Build())
	}

	return e.CreateMessage(ix.EphemeralMessageContent(message).Build())
}

func isBelowMaxActive(e interactionEvent, maxActive int) (bool, error) {
	if maxActive == 0 {
		return true, nil
	}

	if e.GuildID() == nil {
		slog.Error("Cannot determine if below max active modmails: no guild")
		return false, ix.ErrEventNoGuildID
	}

	guildID := *e.GuildID()

	activeThreads, err := e.Client().Rest().GetActiveGuildThreads(guildID)
	if err != nil {
		slog.Error("Failed to retrieve active threads", "err", err)
		return false, fmt.Errorf("unable to retrieve active guild threads: %w", err)
	}

	userThreadsCount := 0

	for _, thread := range activeThreads.Threads {
		if *thread.ParentID() != e.Channel().ID() {
			continue
		}
		members, err := e.Client().Rest().GetThreadMembers(thread.ID())
		if err != nil {
			slog.Error("Failed to get thread members", "err", err)
			return false, fmt.Errorf("couldn't get thread members: %w", err)
		}

		for _, member := range members {
			if member.UserID == e.User().ID {
				userThreadsCount++
			}
			if userThreadsCount >= maxActive {
				return false, nil
			}
		}
	}

	if userThreadsCount >= maxActive {
		return false, nil
	}

	return true, nil
}

type interactionEvent interface {
	Channel() discord.InteractionChannel
	Client() bot.Client
	GuildID() *snowflake.ID
	User() discord.User
}

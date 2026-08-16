package modmail

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r handler.Router) []discord.ApplicationCommandCreate {
	r.Component("/modmail/report-button/{role}/{channel}/{max-active}/{slow-mode}", ModmailReportButtonHandler)
	r.Modal("/modmail/report-modal/{role}/{channel}/{max-active}/{slow-mode}", ModmailReportModalHandler)
	r.Command("/Report Message", ModmailReportMessageHandler)
	r.Modal("/modmail/report-message/{channelID}/{messageID}", ModmailReportMessageModalHandler)

	slashModmailAdmin := ModmailAdmin.Register(r)

	return []discord.ApplicationCommandCreate{slashModmailAdmin, ModmailReportMessageCommand}
}

var ModmailAdmin = rave.Slash("modmail-admin", "Commands for receiving and sending Modmail.").
	AddNameLocalization(discord.LocaleNorwegian, "modmail-admin").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Kommandoer for å motta og sende Modmail.").
	WithDefaultMemberPermissions(discord.PermissionKickMembers).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	AddContexts(discord.InteractionContextTypeGuild).
	AddOptions(createSubCommand, settingsSubCommand)

var createSubCommand = rave.SubCommand("create-button", "Create a Modmail report button.").
	AddNameLocalization(discord.LocaleNorwegian, "opprett-knapp").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Opprett en rapport-knapp for Modmail.").
	AddOptions(
		rave.OptionString("label", "The label to display on the button.").
			AddNameLocalization(discord.LocaleNorwegian, "tekst").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Teksten som skal vises på knappen.").
			WithRequired(true),
		rave.OptionString("button-color", "The color of the button.").
			AddNameLocalization(discord.LocaleNorwegian, "knapp-farge").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Fargen på knappen.").
			WithRequired(false).
			AddChoice("Red", "red").
			AddChoice("Green", "green").
			AddChoice("Blue", "blue").
			AddChoice("Gray", "gray"),
		rave.OptionRole("role", "Role that should be mentioned/notified when a new thread is created.").
			AddNameLocalization(discord.LocaleNorwegian, "rolle").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Rollen som skal nevnes/varsles når en ny tråd opprettes.").
			WithRequired(false),
		rave.OptionChannel("channel", "Channel that notifications should be sent to.").
			AddNameLocalization(discord.LocaleNorwegian, "kanal").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Kanal som varsler skal sendes til.").
			WithRequired(false).
			AddChannelTypes(discord.ChannelTypeGuildText),
		rave.OptionInt("max-active-reports", "The maximum number of active reports that a user can have in the channel.").
			AddNameLocalization(discord.LocaleNorwegian, "maks-aktive-rapporter").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Maksimalt antall aktive rapporter en bruker kan ha i kanalen.").
			WithRequired(false).
			WithMinValue(0).
			WithMaxValue(100),
		rave.OptionString("slow-mode-time", "Enable slow mode for the report thread in the format '1h5m30s1' ('0s' = disabled)").
			AddNameLocalization(discord.LocaleNorwegian, "treg-modus-tid").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Aktiver treg modus for rapport-tråden i formatet '1t5m30s1' ('0s' = deaktivert)").
			WithRequired(false),
	).
	Handle(ModmailAdminCreateButtonHandler)

var settingsSubCommand = rave.SubCommand("settings", "Modmail settings").
	AddNameLocalization(discord.LocaleNorwegian, "innstillinger").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Modmail-innstillinger.").
	AddOptions(
		rave.OptionChannel("report-channel", "Channel that reports will go into. (Does not apply to report buttons)").
			AddNameLocalization(discord.LocaleNorwegian, "rapport-kanal").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Kanal som rapporter vil gå inn i. (Gjelder ikke for rapportknapper)").
			AddChannelTypes(discord.ChannelTypeGuildText),
		rave.OptionRole("report-ping-role", "The role that will be pinged when a report is made").
			AddNameLocalization(discord.LocaleNorwegian, "rapport-varsle-rolle").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Rollen som vil bli varslet når en rapport blir laget."),
		rave.OptionChannel("notification-channel", "Channel that report notifications will be posted to").
			AddNameLocalization(discord.LocaleNorwegian, "varsel-kanal").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Kanal som rapportvarsler vil bli lagt ut i.").
			AddChannelTypes(discord.ChannelTypeGuildText),
		rave.OptionString("reset", "Reset a setting to its default value.").
			AddNameLocalization(discord.LocaleNorwegian, "tilbakestill").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Tilbakestill en innstilling til standardverdien.").
			AddChoice("report-channel", "report-channel").
			AddChoice("notification-channel", "notification-channel").
			AddChoice("report-ping-role", "report-ping-role").
			AddChoice("all", "all"),
	).
	Handle(ModmailSettingsHandler)

func ModmailSettingsHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()

	reportChannel, reportChannelOK := data.OptChannel("report-channel")
	pingRole, pingRoleOK := data.OptRole("report-ping-role")
	notificationChannel, notificationChannelOK := data.OptChannel("notification-channel")
	resetOption, resetOptionOK := data.OptString("reset")

	settings, err := model.GetModmailSettings(*e.GuildID())
	if err != nil {
		slog.Warn("Failed to load Modmail settings", "guild", *e.GuildID())
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to load settings."))
	}

	if !reportChannelOK && !pingRoleOK && !notificationChannelOK && !resetOptionOK {
		reportThreadsChannel := utils.MentionChannelOrDefault(&settings.ReportThreadsChannel, "not set")
		reportNotificationChannel := utils.MentionChannelOrDefault(&settings.ReportNotificationChannel, "not set")
		reportPingRole := utils.MentionRoleOrDefault(&settings.ReportPingRole, "not set")

		return e.CreateMessage(
			ix.EphemeralMessageContentf(
				"## Modmail Settings\n"+
					"**Report Channel:** %s\n"+
					"> Channel report threads will be created in.\n\n"+
					"**Notification Channel:** %s\n"+
					"> Channel that notifications about new report threads will be sent to.\n\n"+
					"**Ping Role:** %s\n"+
					"> Role that will be pinged when a new thread is created.",

				reportThreadsChannel,
				reportNotificationChannel,
				reportPingRole,
			),
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
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to save settings."))
	}

	return e.CreateMessage(ix.EphemeralMessageContent(message))
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

	activeThreads, err := e.Client().Rest.GetActiveGuildThreads(guildID)
	if err != nil {
		slog.Error("Failed to retrieve active threads", "err", err)
		return false, fmt.Errorf("unable to retrieve active guild threads: %w", err)
	}

	userThreadsCount := 0

	for _, thread := range activeThreads.Threads {
		if *thread.ParentID() != e.Channel().ID() {
			continue
		}
		members, err := e.Client().Rest.GetThreadMembers(thread.ID())
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
	Client() *bot.Client
	GuildID() *snowflake.ID
	User() discord.User
}

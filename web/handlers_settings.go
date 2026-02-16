package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

// guildChannels returns a list of channels for the guild from cache.
func guildChannels(client *bot.Client, guildID snowflake.ID) []components.ChannelInfo {
	var channels []components.ChannelInfo
	for ch := range client.Caches.ChannelsForGuild(guildID) {
		var parentID string
		if pid := ch.ParentID(); pid != nil {
			parentID = pid.String()
		}
		channels = append(channels, components.ChannelInfo{
			ID:       ch.ID().String(),
			Name:     ch.Name(),
			Type:     ch.Type(),
			Position: ch.Position(),
			ParentID: parentID,
		})
	}
	return channels
}

// guildRoles returns a list of roles for the guild from cache.
func guildRoles(client *bot.Client, guildID snowflake.ID) []components.RoleInfo {
	var roles []components.RoleInfo
	for role := range client.Caches.Roles(guildID) {
		roles = append(roles, components.RoleInfo{
			ID:       role.ID.String(),
			Name:     role.Name,
			Position: role.Position,
			Managed:  role.Managed,
		})
	}
	return roles
}

func handleDashboard(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			http.Error(w, "failed to load settings", http.StatusInternalServerError)
			return
		}

		ms, err := model.GetModmailSettings(guildID)
		if err != nil {
			http.Error(w, "failed to load modmail settings", http.StatusInternalServerError)
			return
		}

		channels := guildChannels(client, guildID)
		roles := guildRoles(client, guildID)

		guild, _ := client.Caches.Guild(guildID)
		nav := layouts.NavData{
			User:      session,
			GuildID:   guildIDStr,
			GuildName: guild.Name,
		}

		allSections := allSettingsSections(guildIDStr, settings, ms, channels, roles)
		pages.Dashboard(nav, guildIDStr, allSections).Render(r.Context(), w)
	}
}

// allSettingsSections renders all 7 settings sections as a single component.
func allSettingsSections(guildID string, settings *model.GuildSettings, ms *model.ModmailSettings, channels []components.ChannelInfo, roles []components.RoleInfo) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := partials.SettingsModChannel(partials.ModChannelData{
			GuildID:          guildID,
			ModeratorChannel: idStr(settings.ModeratorChannel),
			Channels:         channels,
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsInfractions(partials.InfractionsData{
			GuildID:                     guildID,
			HalfLifeDays:                settings.InfractionHalfLifeDays,
			NotifyOnWarnedUserJoin:      settings.NotifyOnWarnedUserJoin,
			NotifyWarnSeverityThreshold: settings.NotifyWarnSeverityThreshold,
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsGatekeep(partials.GatekeepData{
			GuildID:               guildID,
			Enabled:               settings.GatekeepEnabled,
			PendingRole:           idStr(settings.GatekeepPendingRole),
			ApprovedRole:          idStr(settings.GatekeepApprovedRole),
			AddPendingRoleOnJoin:  settings.GatekeepAddPendingRoleOnJoin,
			ApprovedMessage:       settings.GatekeepApprovedMessage,
			ApprovedMessageV2:     settings.GatekeepApprovedMessageV2,
			ApprovedMessageV2Json: settings.GatekeepApprovedMessageV2Json,
			Roles:                 roles,
			Placeholders:          utils.MessageTemplatePlaceholders,
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsJoinLeave(partials.JoinLeaveData{
			GuildID:             guildID,
			JoinMessageEnabled:  settings.JoinMessageEnabled,
			JoinMessage:         settings.JoinMessage,
			JoinMessageV2:       settings.JoinMessageV2,
			JoinMessageV2Json:   settings.JoinMessageV2Json,
			LeaveMessageEnabled: settings.LeaveMessageEnabled,
			LeaveMessage:        settings.LeaveMessage,
			LeaveMessageV2:      settings.LeaveMessageV2,
			LeaveMessageV2Json:  settings.LeaveMessageV2Json,
			Channel:             idStr(settings.JoinLeaveChannel),
			Channels:            channels,
			Placeholders:        utils.MessageTemplatePlaceholders,
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsAntiSpam(partials.AntiSpamData{
			GuildID:         guildID,
			Enabled:         settings.AntiSpamEnabled,
			Count:           settings.AntiSpamCount,
			CooldownSeconds: settings.AntiSpamCooldownSeconds,
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsBanFooter(partials.BanFooterData{
			GuildID:    guildID,
			Footer:     settings.BanFooter,
			AlwaysSend: settings.AlwaysSendBanFooter,
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsModmail(partials.ModmailData{
			GuildID:                   guildID,
			ReportThreadsChannel:      idStr(ms.ReportThreadsChannel),
			ReportNotificationChannel: idStr(ms.ReportNotificationChannel),
			ReportPingRole:            idStr(ms.ReportPingRole),
			Channels:                  channels,
			Roles:                     roles,
		}).Render(ctx, w); err != nil {
			return err
		}
		return nil
	})
}

// --- POST handlers ---

func handleSaveModChannel(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			partials.SettingsModChannel(partials.ModChannelData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		settings.ModeratorChannel = parseSnowflake(r.FormValue("moderator_channel"))
		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save mod channel settings", "error", err)
			partials.SettingsModChannel(partials.ModChannelData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Channels: guildChannels(client, guildID),
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsModChannel(partials.ModChannelData{
			GuildID:          guildIDStr,
			ModeratorChannel: idStr(settings.ModeratorChannel),
			Channels:         guildChannels(client, guildID),
			SaveSuccess:      true,
		}).Render(r.Context(), w)
	}
}

func handleSaveInfractions(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		r.ParseForm()
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			partials.SettingsInfractions(partials.InfractionsData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		settings.InfractionHalfLifeDays = parseFloat(r.FormValue("half_life_days"))
		settings.NotifyOnWarnedUserJoin = r.FormValue("notify_on_warned_user_join") == "true"
		settings.NotifyWarnSeverityThreshold = parseFloat(r.FormValue("notify_warn_severity_threshold"))

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save infraction settings", "error", err)
			partials.SettingsInfractions(partials.InfractionsData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsInfractions(partials.InfractionsData{
			GuildID:                     guildIDStr,
			HalfLifeDays:                settings.InfractionHalfLifeDays,
			NotifyOnWarnedUserJoin:      settings.NotifyOnWarnedUserJoin,
			NotifyWarnSeverityThreshold: settings.NotifyWarnSeverityThreshold,
			SaveSuccess:                 true,
		}).Render(r.Context(), w)
	}
}

func handleSaveAntiSpam(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		r.ParseForm()
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			partials.SettingsAntiSpam(partials.AntiSpamData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		settings.AntiSpamEnabled = r.FormValue("enabled") == "true"
		settings.AntiSpamCount = parseInt(r.FormValue("count"), 5)
		settings.AntiSpamCooldownSeconds = parseInt(r.FormValue("cooldown_seconds"), 20)

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save anti-spam settings", "error", err)
			partials.SettingsAntiSpam(partials.AntiSpamData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsAntiSpam(partials.AntiSpamData{
			GuildID:         guildIDStr,
			Enabled:         settings.AntiSpamEnabled,
			Count:           settings.AntiSpamCount,
			CooldownSeconds: settings.AntiSpamCooldownSeconds,
			SaveSuccess:     true,
		}).Render(r.Context(), w)
	}
}

func handleSaveBanFooter(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		r.ParseForm()
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			partials.SettingsBanFooter(partials.BanFooterData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		settings.BanFooter = r.FormValue("footer")
		settings.AlwaysSendBanFooter = r.FormValue("always_send") == "true"

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save ban footer settings", "error", err)
			partials.SettingsBanFooter(partials.BanFooterData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsBanFooter(partials.BanFooterData{
			GuildID:     guildIDStr,
			Footer:      settings.BanFooter,
			AlwaysSend:  settings.AlwaysSendBanFooter,
			SaveSuccess: true,
		}).Render(r.Context(), w)
	}
}

func handleSaveModmail(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		r.ParseForm()
		ms, err := model.GetModmailSettings(guildID)
		if err != nil {
			partials.SettingsModmail(partials.ModmailData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		ms.ReportThreadsChannel = parseSnowflake(r.FormValue("report_threads_channel"))
		ms.ReportNotificationChannel = parseSnowflake(r.FormValue("report_notification_channel"))
		ms.ReportPingRole = parseSnowflake(r.FormValue("report_ping_role"))

		if err := model.SetModmailSettings(ms); err != nil {
			slog.Error("failed to save modmail settings", "error", err)
			partials.SettingsModmail(partials.ModmailData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Channels: guildChannels(client, guildID),
				Roles:    guildRoles(client, guildID),
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsModmail(partials.ModmailData{
			GuildID:                   guildIDStr,
			ReportThreadsChannel:      idStr(ms.ReportThreadsChannel),
			ReportNotificationChannel: idStr(ms.ReportNotificationChannel),
			ReportPingRole:            idStr(ms.ReportPingRole),
			Channels:                  guildChannels(client, guildID),
			Roles:                     guildRoles(client, guildID),
			SaveSuccess:               true,
		}).Render(r.Context(), w)
	}
}

func handleSaveGatekeep(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		r.ParseForm()
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			partials.SettingsGatekeep(partials.GatekeepData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		settings.GatekeepEnabled = r.FormValue("enabled") == "true"
		settings.GatekeepPendingRole = parseSnowflake(r.FormValue("pending_role"))
		settings.GatekeepApprovedRole = parseSnowflake(r.FormValue("approved_role"))
		settings.GatekeepAddPendingRoleOnJoin = r.FormValue("add_pending_role_on_join") == "true"
		settings.GatekeepApprovedMessage = r.FormValue("approved_message")
		settings.GatekeepApprovedMessageV2 = r.FormValue("approved_message_v2") == "true"
		settings.GatekeepApprovedMessageV2Json = r.FormValue("approved_message_v2_json")

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save gatekeep settings", "error", err)
			partials.SettingsGatekeep(partials.GatekeepData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Roles:   guildRoles(client, guildID),
				Placeholders: utils.MessageTemplatePlaceholders,
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsGatekeep(partials.GatekeepData{
			GuildID:               guildIDStr,
			Enabled:               settings.GatekeepEnabled,
			PendingRole:           idStr(settings.GatekeepPendingRole),
			ApprovedRole:          idStr(settings.GatekeepApprovedRole),
			AddPendingRoleOnJoin:  settings.GatekeepAddPendingRoleOnJoin,
			ApprovedMessage:       settings.GatekeepApprovedMessage,
			ApprovedMessageV2:     settings.GatekeepApprovedMessageV2,
			ApprovedMessageV2Json: settings.GatekeepApprovedMessageV2Json,
			Roles:                 guildRoles(client, guildID),
			Placeholders:          utils.MessageTemplatePlaceholders,
			SaveSuccess:           true,
		}).Render(r.Context(), w)
	}
}

func handleSaveJoinLeave(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		r.ParseForm()
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			partials.SettingsJoinLeave(partials.JoinLeaveData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}).Render(r.Context(), w)
			return
		}

		settings.JoinMessageEnabled = r.FormValue("join_message_enabled") == "true"
		settings.JoinMessage = r.FormValue("join_message")
		settings.JoinMessageV2 = r.FormValue("join_message_v2") == "true"
		settings.JoinMessageV2Json = r.FormValue("join_message_v2_json")
		settings.LeaveMessageEnabled = r.FormValue("leave_message_enabled") == "true"
		settings.LeaveMessage = r.FormValue("leave_message")
		settings.LeaveMessageV2 = r.FormValue("leave_message_v2") == "true"
		settings.LeaveMessageV2Json = r.FormValue("leave_message_v2_json")
		settings.JoinLeaveChannel = parseSnowflake(r.FormValue("channel"))

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save join/leave settings", "error", err)
			partials.SettingsJoinLeave(partials.JoinLeaveData{
				GuildID:  guildIDStr, SaveError: "Failed to save settings.",
				Channels: guildChannels(client, guildID),
				Placeholders: utils.MessageTemplatePlaceholders,
			}).Render(r.Context(), w)
			return
		}

		partials.SettingsJoinLeave(partials.JoinLeaveData{
			GuildID:             guildIDStr,
			JoinMessageEnabled:  settings.JoinMessageEnabled,
			JoinMessage:         settings.JoinMessage,
			JoinMessageV2:       settings.JoinMessageV2,
			JoinMessageV2Json:   settings.JoinMessageV2Json,
			LeaveMessageEnabled: settings.LeaveMessageEnabled,
			LeaveMessage:        settings.LeaveMessage,
			LeaveMessageV2:      settings.LeaveMessageV2,
			LeaveMessageV2Json:  settings.LeaveMessageV2Json,
			Channel:             idStr(settings.JoinLeaveChannel),
			Channels:            guildChannels(client, guildID),
			Placeholders:        utils.MessageTemplatePlaceholders,
			SaveSuccess:         true,
		}).Render(r.Context(), w)
	}
}

// --- Helpers ---

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string, defaultVal int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

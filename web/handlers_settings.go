package web

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

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

// guildChannels returns a list of channels for the guild from cache, sorted
// by parent category, then channel position, then name. The cache iterator
// has non-deterministic order, so without sorting the dropdown order would
// change between page loads.
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
	slices.SortStableFunc(channels, func(a, b components.ChannelInfo) int {
		if c := cmp.Compare(a.ParentID, b.ParentID); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Position, b.Position); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return channels
}

// guildRoles returns a list of roles for the guild from cache, sorted to
// match Discord's display order: highest position first, then by name. The
// cache iterator has non-deterministic order.
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
	slices.SortStableFunc(roles, func(a, b components.RoleInfo) int {
		if c := cmp.Compare(b.Position, a.Position); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
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
		renderSafe(w, r, pages.Dashboard(nav, guildIDStr, allSections))
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
			renderSafe(w, r, partials.SettingsModChannel(partials.ModChannelData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		settings.ModeratorChannel = parseSnowflake(r.FormValue("moderator_channel"))
		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save mod channel settings", "error", err)
			renderSafe(w, r, partials.SettingsModChannel(partials.ModChannelData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Channels: guildChannels(client, guildID),
			}))
			return
		}

		renderSafe(w, r, partials.SettingsModChannel(partials.ModChannelData{
			GuildID:          guildIDStr,
			ModeratorChannel: idStr(settings.ModeratorChannel),
			Channels:         guildChannels(client, guildID),
			SaveSuccess:      true,
		}))
	}
}

func handleSaveInfractions(client *bot.Client) http.HandlerFunc {
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
			renderSafe(w, r, partials.SettingsInfractions(partials.InfractionsData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		settings.InfractionHalfLifeDays = parseFloat(r.FormValue("half_life_days"))
		settings.NotifyOnWarnedUserJoin = r.FormValue("notify_on_warned_user_join") == "true"
		settings.NotifyWarnSeverityThreshold = parseFloat(r.FormValue("notify_warn_severity_threshold"))

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save infraction settings", "error", err)
			renderSafe(w, r, partials.SettingsInfractions(partials.InfractionsData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
			}))
			return
		}

		renderSafe(w, r, partials.SettingsInfractions(partials.InfractionsData{
			GuildID:                     guildIDStr,
			HalfLifeDays:                settings.InfractionHalfLifeDays,
			NotifyOnWarnedUserJoin:      settings.NotifyOnWarnedUserJoin,
			NotifyWarnSeverityThreshold: settings.NotifyWarnSeverityThreshold,
			SaveSuccess:                 true,
		}))
	}
}

func handleSaveAntiSpam(client *bot.Client) http.HandlerFunc {
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
			renderSafe(w, r, partials.SettingsAntiSpam(partials.AntiSpamData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		settings.AntiSpamEnabled = r.FormValue("enabled") == "true"
		settings.AntiSpamCount = parseInt(r.FormValue("count"), 5)
		settings.AntiSpamCooldownSeconds = parseInt(r.FormValue("cooldown_seconds"), 20)

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save anti-spam settings", "error", err)
			renderSafe(w, r, partials.SettingsAntiSpam(partials.AntiSpamData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
			}))
			return
		}

		renderSafe(w, r, partials.SettingsAntiSpam(partials.AntiSpamData{
			GuildID:         guildIDStr,
			Enabled:         settings.AntiSpamEnabled,
			Count:           settings.AntiSpamCount,
			CooldownSeconds: settings.AntiSpamCooldownSeconds,
			SaveSuccess:     true,
		}))
	}
}

func handleSaveBanFooter(client *bot.Client) http.HandlerFunc {
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
			renderSafe(w, r, partials.SettingsBanFooter(partials.BanFooterData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		settings.BanFooter = r.FormValue("footer")
		settings.AlwaysSendBanFooter = r.FormValue("always_send") == "true"

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save ban footer settings", "error", err)
			renderSafe(w, r, partials.SettingsBanFooter(partials.BanFooterData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
			}))
			return
		}

		renderSafe(w, r, partials.SettingsBanFooter(partials.BanFooterData{
			GuildID:     guildIDStr,
			Footer:      settings.BanFooter,
			AlwaysSend:  settings.AlwaysSendBanFooter,
			SaveSuccess: true,
		}))
	}
}

func handleSaveModmail(client *bot.Client) http.HandlerFunc {
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
		ms, err := model.GetModmailSettings(guildID)
		if err != nil {
			renderSafe(w, r, partials.SettingsModmail(partials.ModmailData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		ms.ReportThreadsChannel = parseSnowflake(r.FormValue("report_threads_channel"))
		ms.ReportNotificationChannel = parseSnowflake(r.FormValue("report_notification_channel"))
		ms.ReportPingRole = parseSnowflake(r.FormValue("report_ping_role"))

		if err := model.SetModmailSettings(ms); err != nil {
			slog.Error("failed to save modmail settings", "error", err)
			renderSafe(w, r, partials.SettingsModmail(partials.ModmailData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Channels: guildChannels(client, guildID),
				Roles:    guildRoles(client, guildID),
			}))
			return
		}

		renderSafe(w, r, partials.SettingsModmail(partials.ModmailData{
			GuildID:                   guildIDStr,
			ReportThreadsChannel:      idStr(ms.ReportThreadsChannel),
			ReportNotificationChannel: idStr(ms.ReportNotificationChannel),
			ReportPingRole:            idStr(ms.ReportPingRole),
			Channels:                  guildChannels(client, guildID),
			Roles:                     guildRoles(client, guildID),
			SaveSuccess:               true,
		}))
	}
}

func handleSaveGatekeep(client *bot.Client) http.HandlerFunc {
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
			renderSafe(w, r, partials.SettingsGatekeep(partials.GatekeepData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		settings.GatekeepEnabled = r.FormValue("enabled") == "true"
		settings.GatekeepPendingRole = parseSnowflake(r.FormValue("pending_role"))
		settings.GatekeepApprovedRole = parseSnowflake(r.FormValue("approved_role"))
		settings.GatekeepAddPendingRoleOnJoin = r.FormValue("add_pending_role_on_join") == "true"
		settings.GatekeepApprovedMessage = r.FormValue("approved_message")
		settings.GatekeepApprovedMessageV2 = r.FormValue("approved_message_v2") == "true"
		approvedV2Raw := r.FormValue("approved_message_v2_json")
		if settings.GatekeepApprovedMessageV2 {
			compact, err := validateAndCompactV2JSON(approvedV2Raw)
			if err != nil {
				renderSafe(w, r, partials.SettingsGatekeep(partials.GatekeepData{
					GuildID:               guildIDStr,
					Enabled:               settings.GatekeepEnabled,
					PendingRole:           idStr(settings.GatekeepPendingRole),
					ApprovedRole:          idStr(settings.GatekeepApprovedRole),
					AddPendingRoleOnJoin:  settings.GatekeepAddPendingRoleOnJoin,
					ApprovedMessage:       settings.GatekeepApprovedMessage,
					ApprovedMessageV2:     settings.GatekeepApprovedMessageV2,
					ApprovedMessageV2Json: approvedV2Raw,
					Roles:                 guildRoles(client, guildID),
					Placeholders:          utils.MessageTemplatePlaceholders,
					SaveError:             "Approved message: " + err.Error() + ".",
				}))
				return
			}
			settings.GatekeepApprovedMessageV2Json = compact
		} else {
			settings.GatekeepApprovedMessageV2Json = approvedV2Raw
		}

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save gatekeep settings", "error", err)
			renderSafe(w, r, partials.SettingsGatekeep(partials.GatekeepData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Roles:        guildRoles(client, guildID),
				Placeholders: utils.MessageTemplatePlaceholders,
			}))
			return
		}

		renderSafe(w, r, partials.SettingsGatekeep(partials.GatekeepData{
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
		}))
	}
}

func handleSaveJoinLeave(client *bot.Client) http.HandlerFunc {
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
			renderSafe(w, r, partials.SettingsJoinLeave(partials.JoinLeaveData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		settings.JoinMessageEnabled = r.FormValue("join_message_enabled") == "true"
		settings.JoinMessage = r.FormValue("join_message")
		settings.JoinMessageV2 = r.FormValue("join_message_v2") == "true"
		joinV2Raw := r.FormValue("join_message_v2_json")
		settings.LeaveMessageEnabled = r.FormValue("leave_message_enabled") == "true"
		settings.LeaveMessage = r.FormValue("leave_message")
		settings.LeaveMessageV2 = r.FormValue("leave_message_v2") == "true"
		leaveV2Raw := r.FormValue("leave_message_v2_json")
		settings.JoinLeaveChannel = parseSnowflake(r.FormValue("channel"))

		renderJoinLeaveError := func(message string) {
			renderSafe(w, r, partials.SettingsJoinLeave(partials.JoinLeaveData{
				GuildID:             guildIDStr,
				JoinMessageEnabled:  settings.JoinMessageEnabled,
				JoinMessage:         settings.JoinMessage,
				JoinMessageV2:       settings.JoinMessageV2,
				JoinMessageV2Json:   joinV2Raw,
				LeaveMessageEnabled: settings.LeaveMessageEnabled,
				LeaveMessage:        settings.LeaveMessage,
				LeaveMessageV2:      settings.LeaveMessageV2,
				LeaveMessageV2Json:  leaveV2Raw,
				Channel:             idStr(settings.JoinLeaveChannel),
				Channels:            guildChannels(client, guildID),
				Placeholders:        utils.MessageTemplatePlaceholders,
				SaveError:           message,
			}))
		}

		if settings.JoinMessageV2 {
			compact, err := validateAndCompactV2JSON(joinV2Raw)
			if err != nil {
				renderJoinLeaveError("Join message: " + err.Error() + ".")
				return
			}
			settings.JoinMessageV2Json = compact
		} else {
			settings.JoinMessageV2Json = joinV2Raw
		}

		if settings.LeaveMessageV2 {
			compact, err := validateAndCompactV2JSON(leaveV2Raw)
			if err != nil {
				renderJoinLeaveError("Leave message: " + err.Error() + ".")
				return
			}
			settings.LeaveMessageV2Json = compact
		} else {
			settings.LeaveMessageV2Json = leaveV2Raw
		}

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save join/leave settings", "error", err)
			renderSafe(w, r, partials.SettingsJoinLeave(partials.JoinLeaveData{
				GuildID: guildIDStr, SaveError: "Failed to save settings.",
				Channels:     guildChannels(client, guildID),
				Placeholders: utils.MessageTemplatePlaceholders,
			}))
			return
		}

		renderSafe(w, r, partials.SettingsJoinLeave(partials.JoinLeaveData{
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
		}))
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

// errInvalidV2JSON is returned by validateAndCompactV2JSON when the supplied
// payload is missing, malformed, or not a JSON array of components.
var errInvalidV2JSON = errors.New("V2 components JSON must be a non-empty JSON array")

// validateAndCompactV2JSON validates that s is a JSON array (the shape
// expected by utils.BuildV2Message) and returns a compact, canonical form
// safe to embed in HTML attributes. Empty/whitespace input is rejected so
// callers can surface a clear error before persisting.
func validateAndCompactV2JSON(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errInvalidV2JSON
	}
	var parsed []any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return "", errInvalidV2JSON
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

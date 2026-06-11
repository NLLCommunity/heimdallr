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
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

// guildChannels reads channels for the guild from the cache and returns them
// grouped by category in Discord's display order. The cache iterator is
// non-deterministic, so without explicit ordering the dropdown order would
// change between page loads. Pure ordering logic lives in groupChannels so
// it can be unit tested without a live bot client.
func guildChannels(client *bot.Client, guildID snowflake.ID) []components.ChannelGroup {
	var all []components.ChannelInfo
	for ch := range client.Caches.ChannelsForGuild(guildID) {
		info := components.ChannelInfo{
			ID:       ch.ID().String(),
			Name:     ch.Name(),
			Type:     ch.Type(),
			Position: ch.Position(),
		}
		if pid := ch.ParentID(); pid != nil {
			info.ParentID = pid.String()
		}
		all = append(all, info)
	}
	return groupChannels(all)
}

// groupChannels takes a flat list of channels (categories included) and
// returns text channels grouped by category in Discord's display order:
// uncategorized channels first, then each category (ordered by position)
// followed by its text children (ordered by position). Name is the
// tiebreaker at every level. Categories with no text children are omitted;
// a channel whose ParentID does not match any category in the input is
// treated as uncategorized rather than producing an empty-label optgroup.
func groupChannels(all []components.ChannelInfo) []components.ChannelGroup {
	type category struct {
		name     string
		position int
		channels []components.ChannelInfo
	}
	categories := map[string]*category{}
	for _, c := range all {
		if c.Type == discord.ChannelTypeGuildCategory {
			cat, ok := categories[c.ID]
			if !ok {
				cat = &category{}
				categories[c.ID] = cat
			}
			cat.name = c.Name
			cat.position = c.Position
		}
	}

	var topLevel []components.ChannelInfo
	for _, c := range all {
		if !components.IsTextChannel(c.Type) {
			continue
		}
		if c.ParentID == "" {
			topLevel = append(topLevel, c)
			continue
		}
		cat, ok := categories[c.ParentID]
		if !ok {
			topLevel = append(topLevel, c)
			continue
		}
		cat.channels = append(cat.channels, c)
	}

	byPosition := func(a, b components.ChannelInfo) int {
		if c := cmp.Compare(a.Position, b.Position); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	}
	slices.SortStableFunc(topLevel, byPosition)

	type sortedCat struct {
		name     string
		position int
		channels []components.ChannelInfo
	}
	cats := make([]sortedCat, 0, len(categories))
	for _, c := range categories {
		if len(c.channels) == 0 {
			continue
		}
		slices.SortStableFunc(c.channels, byPosition)
		cats = append(cats, sortedCat{name: c.name, position: c.position, channels: c.channels})
	}
	slices.SortStableFunc(cats, func(a, b sortedCat) int {
		if c := cmp.Compare(a.position, b.position); c != 0 {
			return c
		}
		return cmp.Compare(a.name, b.name)
	})

	groups := make([]components.ChannelGroup, 0, len(cats)+1)
	if len(topLevel) > 0 {
		groups = append(groups, components.ChannelGroup{Channels: topLevel})
	}
	for _, c := range cats {
		groups = append(groups, components.ChannelGroup{Name: c.name, Channels: c.channels})
	}
	return groups
}

// rolesWithoutEveryone removes the @everyone role from the list. The
// @everyone role's ID equals the guild's ID, but @everyone is never
// present in member.RoleIDs — picking it as a permission role silently
// grants access to no one. Used by settings panels where the chosen
// role gates access via a RoleIDs lookup (notably posts).
//
// guildIDStr is taken as a string so callers can pass the route-bound
// path value directly without parsing.
func rolesWithoutEveryone(roles []components.RoleInfo, guildIDStr string) []components.RoleInfo {
	out := make([]components.RoleInfo, 0, len(roles))
	for _, r := range roles {
		if r.ID == guildIDStr {
			continue
		}
		out = append(out, r)
	}
	return out
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

		// If the user is a post-mod but not an admin, send them to /posts
		// rather than 403'ing — they have *some* access to this guild.
		// Owner-shortcut + a single guildMember lookup feeds the shared
		// access resolver.
		if parsedID, err := snowflake.Parse(guildIDStr); err == nil {
			if guild, ok := client.Caches.Guild(parsedID); ok && session != nil &&
				guild.OwnerID != session.UserID {
				member := guildMember(client, parsedID, session.UserID)
				if guildAccessLevel(client, guild, member) == guildAccessPosts {
					http.Redirect(w, r, "/guild/"+parsedID.String()+"/posts", http.StatusSeeOther)
					return
				}
			}
		}

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
			IsAdmin:   true,
			IsPostMod: true,
		}

		allSections := allSettingsSections(guildIDStr, settings, ms, channels, roles)
		renderSafe(w, r, pages.Dashboard(nav, guildIDStr, allSections))
	}
}

// allSettingsSections renders all 7 settings sections as a single component.
func allSettingsSections(guildID string, settings *model.GuildSettings, ms *model.ModmailSettings, channels []components.ChannelGroup, roles []components.RoleInfo) templ.Component {
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
		if err := partials.SettingsPosts(partials.PostsData{
			GuildID: guildID,
			ModRole: idStr(settings.PostsModRoleID),
			// @everyone (whose role ID equals the guild ID) is filtered out:
			// it's never present in member.RoleIDs, so picking it would
			// silently grant access to no one. See hasPostsModRole.
			Roles: rolesWithoutEveryone(roles, guildID),
		}).Render(ctx, w); err != nil {
			return err
		}
		if err := partials.SettingsAuditLog(buildAuditLogSettingsData(guildID, settings)).Render(ctx, w); err != nil {
			return err
		}
		return nil
	})
}

// --- POST handlers ---

// handleSavePosts persists the per-guild PostsModRoleID. Admin-gated:
// only admins can choose who gets non-admin posts access, otherwise a
// post-mod could escalate themselves by changing the role to one of
// their own.
func handleSavePosts(client *bot.Client) http.HandlerFunc {
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
			renderSafe(w, r, partials.SettingsPosts(partials.PostsData{
				GuildID: guildIDStr, SaveError: "Failed to load settings.",
			}))
			return
		}

		renderPostsError := func(message string) {
			renderSafe(w, r, partials.SettingsPosts(partials.PostsData{
				GuildID:   guildIDStr,
				ModRole:   idStr(settings.PostsModRoleID),
				Roles:     rolesWithoutEveryone(guildRoles(client, guildID), guildIDStr),
				SaveError: message,
			}))
		}

		modRole, err := parseSnowflakeOrZero(r.FormValue("mod_role"))
		if err != nil {
			renderPostsError("Invalid role ID.")
			return
		}
		// The UI filters @everyone out, but defend-in-depth at the save
		// layer too in case a hand-crafted POST sneaks it past. The
		// shared model check keeps this rule and its message in lockstep
		// with the /admin posts command.
		if err := model.ValidatePostsModRole(guildID, modRole); err != nil {
			renderPostsError(err.Error())
			return
		}
		settings.PostsModRoleID = modRole
		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save posts settings", "error", err)
			renderPostsError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "posts",
			map[string]any{"mod_role": idStr(settings.PostsModRoleID)})

		renderSafe(w, r, partials.SettingsPosts(partials.PostsData{
			GuildID:     guildIDStr,
			ModRole:     idStr(settings.PostsModRoleID),
			Roles:       rolesWithoutEveryone(guildRoles(client, guildID), guildIDStr),
			SaveSuccess: true,
		}))
	}
}

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

		renderModChannelError := func(message string) {
			renderSafe(w, r, partials.SettingsModChannel(partials.ModChannelData{
				GuildID:          guildIDStr,
				ModeratorChannel: idStr(settings.ModeratorChannel),
				Channels:         guildChannels(client, guildID),
				SaveError:        message,
			}))
		}

		modChannel, err := parseSnowflakeOrZero(r.FormValue("moderator_channel"))
		if err != nil {
			renderModChannelError("Invalid channel ID.")
			return
		}
		settings.ModeratorChannel = modChannel
		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save mod channel settings", "error", err)
			renderModChannelError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "mod_channel",
			map[string]any{"moderator_channel": idStr(settings.ModeratorChannel)})

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

		renderInfractionsError := func(message string) {
			renderSafe(w, r, partials.SettingsInfractions(partials.InfractionsData{
				GuildID:                     guildIDStr,
				HalfLifeDays:                settings.InfractionHalfLifeDays,
				NotifyOnWarnedUserJoin:      settings.NotifyOnWarnedUserJoin,
				NotifyWarnSeverityThreshold: settings.NotifyWarnSeverityThreshold,
				SaveError:                   message,
			}))
		}

		halfLife, err := parseFloat(r.FormValue("half_life_days"))
		if err != nil || halfLife < minInfractionHalfLifeDays || halfLife > maxInfractionHalfLifeDays {
			renderInfractionsError("Half-life must be between 0 and 365 days.")
			return
		}
		threshold, err := parseFloat(r.FormValue("notify_warn_severity_threshold"))
		if err != nil || threshold < minNotifyWarnSeverityThreshold || threshold > maxNotifyWarnSeverityThreshold {
			renderInfractionsError("Severity threshold must be between 0 and 100.")
			return
		}
		settings.InfractionHalfLifeDays = halfLife
		settings.NotifyOnWarnedUserJoin = r.FormValue("notify_on_warned_user_join") == "true"
		settings.NotifyWarnSeverityThreshold = threshold

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save infraction settings", "error", err)
			renderInfractionsError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "infractions", map[string]any{
			"half_life_days":                 settings.InfractionHalfLifeDays,
			"notify_on_warned_user_join":     settings.NotifyOnWarnedUserJoin,
			"notify_warn_severity_threshold": settings.NotifyWarnSeverityThreshold,
		})

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

		renderAntiSpamError := func(message string) {
			renderSafe(w, r, partials.SettingsAntiSpam(partials.AntiSpamData{
				GuildID:         guildIDStr,
				Enabled:         settings.AntiSpamEnabled,
				Count:           settings.AntiSpamCount,
				CooldownSeconds: settings.AntiSpamCooldownSeconds,
				SaveError:       message,
			}))
		}

		count := parseInt(r.FormValue("count"), 5)
		if count < minAntiSpamCount || count > maxAntiSpamCount {
			renderAntiSpamError("Message count must be between 2 and 10.")
			return
		}
		cooldown := parseInt(r.FormValue("cooldown_seconds"), 20)
		if cooldown < minAntiSpamCooldownSeconds || cooldown > maxAntiSpamCooldownSeconds {
			renderAntiSpamError("Cooldown must be between 1 and 60 seconds.")
			return
		}
		settings.AntiSpamEnabled = r.FormValue("enabled") == "true"
		settings.AntiSpamCount = count
		settings.AntiSpamCooldownSeconds = cooldown

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save anti-spam settings", "error", err)
			renderAntiSpamError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "anti_spam", map[string]any{
			"enabled":          settings.AntiSpamEnabled,
			"count":            settings.AntiSpamCount,
			"cooldown_seconds": settings.AntiSpamCooldownSeconds,
		})

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
				GuildID:    guildIDStr,
				Footer:     settings.BanFooter,
				AlwaysSend: settings.AlwaysSendBanFooter,
				SaveError:  "Failed to save settings.",
			}))
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "ban_footer", map[string]any{
			"footer":      settings.BanFooter,
			"always_send": settings.AlwaysSendBanFooter,
		})

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

		renderModmailError := func(message string) {
			renderSafe(w, r, partials.SettingsModmail(partials.ModmailData{
				GuildID:                   guildIDStr,
				ReportThreadsChannel:      idStr(ms.ReportThreadsChannel),
				ReportNotificationChannel: idStr(ms.ReportNotificationChannel),
				ReportPingRole:            idStr(ms.ReportPingRole),
				Channels:                  guildChannels(client, guildID),
				Roles:                     guildRoles(client, guildID),
				SaveError:                 message,
			}))
		}

		reportThreadsChannel, err := parseSnowflakeOrZero(r.FormValue("report_threads_channel"))
		if err != nil {
			renderModmailError("Invalid channel ID.")
			return
		}
		reportNotificationChannel, err := parseSnowflakeOrZero(r.FormValue("report_notification_channel"))
		if err != nil {
			renderModmailError("Invalid channel ID.")
			return
		}
		reportPingRole, err := parseSnowflakeOrZero(r.FormValue("report_ping_role"))
		if err != nil {
			renderModmailError("Invalid role ID.")
			return
		}
		ms.ReportThreadsChannel = reportThreadsChannel
		ms.ReportNotificationChannel = reportNotificationChannel
		ms.ReportPingRole = reportPingRole

		if err := model.SetModmailSettings(ms); err != nil {
			slog.Error("failed to save modmail settings", "error", err)
			renderModmailError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "modmail", map[string]any{
			"report_threads_channel":      idStr(ms.ReportThreadsChannel),
			"report_notification_channel": idStr(ms.ReportNotificationChannel),
			"report_ping_role":            idStr(ms.ReportPingRole),
		})

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
		approvedV2Raw := r.FormValue("approved_message_v2_json")

		// Closure captures `settings` and `approvedV2Raw` by reference, so the
		// rendered partial reflects whatever state has been applied at the
		// point of the error — old DB values for early parse errors, the
		// would-be-saved values once form fields have been assigned.
		renderGatekeepError := func(message string) {
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
				SaveError:             message,
			}))
		}

		pendingRole, err := parseSnowflakeOrZero(r.FormValue("pending_role"))
		if err != nil {
			renderGatekeepError("Invalid role ID.")
			return
		}
		approvedRole, err := parseSnowflakeOrZero(r.FormValue("approved_role"))
		if err != nil {
			renderGatekeepError("Invalid role ID.")
			return
		}
		settings.GatekeepPendingRole = pendingRole
		settings.GatekeepApprovedRole = approvedRole
		settings.GatekeepAddPendingRoleOnJoin = r.FormValue("add_pending_role_on_join") == "true"
		settings.GatekeepApprovedMessage = r.FormValue("approved_message")
		settings.GatekeepApprovedMessageV2 = r.FormValue("approved_message_v2") == "true"
		if settings.GatekeepApprovedMessageV2 {
			compact, err := validateAndCompactV2JSON(approvedV2Raw)
			if err != nil {
				renderGatekeepError("Approved message: " + err.Error() + ".")
				return
			}
			settings.GatekeepApprovedMessageV2Json = compact
		} else {
			settings.GatekeepApprovedMessageV2Json = preserveV2Json(approvedV2Raw)
		}

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save gatekeep settings", "error", err)
			renderGatekeepError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "gatekeep", map[string]any{
			"enabled":                  settings.GatekeepEnabled,
			"pending_role":             idStr(settings.GatekeepPendingRole),
			"approved_role":            idStr(settings.GatekeepApprovedRole),
			"add_pending_role_on_join": settings.GatekeepAddPendingRoleOnJoin,
			"approved_message_v2":      settings.GatekeepApprovedMessageV2,
		})

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

		channel, err := parseSnowflakeOrZero(r.FormValue("channel"))
		if err != nil {
			renderJoinLeaveError("Invalid channel ID.")
			return
		}
		settings.JoinLeaveChannel = channel

		if settings.JoinMessageV2 {
			compact, err := validateAndCompactV2JSON(joinV2Raw)
			if err != nil {
				renderJoinLeaveError("Join message: " + err.Error() + ".")
				return
			}
			settings.JoinMessageV2Json = compact
		} else {
			settings.JoinMessageV2Json = preserveV2Json(joinV2Raw)
		}

		if settings.LeaveMessageV2 {
			compact, err := validateAndCompactV2JSON(leaveV2Raw)
			if err != nil {
				renderJoinLeaveError("Leave message: " + err.Error() + ".")
				return
			}
			settings.LeaveMessageV2Json = compact
		} else {
			settings.LeaveMessageV2Json = preserveV2Json(leaveV2Raw)
		}

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save join/leave settings", "error", err)
			renderJoinLeaveError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "join_leave", map[string]any{
			"join_message_enabled":  settings.JoinMessageEnabled,
			"leave_message_enabled": settings.LeaveMessageEnabled,
			"join_message_v2":       settings.JoinMessageV2,
			"leave_message_v2":      settings.LeaveMessageV2,
			"join_leave_channel":    idStr(settings.JoinLeaveChannel),
		})

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

// Numeric setting bounds. Mirror the min/max passed to NumberField in the
// templ partials — keep them in sync so client-side HTML5 validation and
// server-side validation agree.
const (
	minAntiSpamCount               = 2
	maxAntiSpamCount               = 10
	minAntiSpamCooldownSeconds     = 1
	maxAntiSpamCooldownSeconds     = 60
	minInfractionHalfLifeDays      = 0.0
	maxInfractionHalfLifeDays      = 365.0
	minNotifyWarnSeverityThreshold = 0.0
	maxNotifyWarnSeverityThreshold = 100.0
	// Cap on raw V2 JSON kept in the DB when the V2 toggle is off (the
	// user's in-flight draft). Real Discord component payloads are
	// kilobytes; 32 KiB leaves headroom without letting unbounded garbage
	// accumulate per-guild.
	maxV2JsonStored = 32 * 1024
)

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func parseInt(s string, defaultVal int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return defaultVal
	}
	return v
}

// preserveV2Json returns a compacted form of raw if it parses as a JSON
// array and is within size limits, else "". Used when the V2 toggle is off
// to keep the user's in-flight draft across visits without persisting
// malformed or oversized payloads.
func preserveV2Json(raw string) string {
	if raw == "" || len(raw) > maxV2JsonStored {
		return ""
	}
	compact, err := validateAndCompactV2JSON(raw)
	if err != nil {
		return ""
	}
	return compact
}

// errInvalidV2JSON is returned by validateAndCompactV2JSON when the supplied
// payload is missing, malformed, or not a JSON array of components.
var errInvalidV2JSON = errors.New("V2 components JSON must be a non-empty JSON array")

// validateAndCompactV2JSON validates that s is a non-empty JSON array (the
// shape expected by utils.BuildV2Message) and returns a compact, canonical
// form safe to embed in HTML attributes. Empty/whitespace input is rejected
// so callers can surface a clear error before persisting.
//
// Decoding into []json.RawMessage rather than []any preserves any large
// integer values verbatim — decoding into []any would coerce JSON numbers
// to float64 and silently truncate 64-bit precision (e.g. snowflakes).
func validateAndCompactV2JSON(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errInvalidV2JSON
	}
	var parsed []json.RawMessage
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return "", errInvalidV2JSON
	}
	if len(parsed) == 0 {
		return "", errInvalidV2JSON
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

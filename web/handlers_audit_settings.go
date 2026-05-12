package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/spf13/viper"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/scheduled_tasks"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

// buildAuditLogSettingsData fills the partial data struct from current
// settings + config-derived ceilings. The "override" fields show the
// guild's current explicit value (or empty when using defaults), so that
// re-rendering preserves what the user has saved.
func buildAuditLogSettingsData(guildID string, settings *model.GuildSettings) partials.AuditLogSettingsData {
	maxMessage := uintFromConfig("audit_log.message_retention_days")
	maxMember := uintFromConfig("audit_log.member_retention_days")
	maxGuild := uintFromConfig("audit_log.guild_retention_days")

	return partials.AuditLogSettingsData{
		GuildID: guildID,
		Enabled: settings.AuditLogEnabled,

		MessageRetentionDaysOverride: retentionOverrideDisplay(settings.AuditMessageRetentionDays, maxMessage),
		MemberRetentionDaysOverride:  retentionOverrideDisplay(settings.AuditMemberRetentionDays, maxMember),
		GuildRetentionDaysOverride:   retentionOverrideDisplay(settings.AuditGuildRetentionDays, maxGuild),

		MaxMessageRetentionDays: maxMessage,
		MaxMemberRetentionDays:  maxMember,
		MaxGuildRetentionDays:   maxGuild,

		EffectiveMessageRetentionDays: effectiveRetentionLabel("audit_log.message_retention_days", settings.AuditMessageRetentionDays),
		EffectiveMemberRetentionDays:  effectiveRetentionLabel("audit_log.member_retention_days", settings.AuditMemberRetentionDays),
		EffectiveGuildRetentionDays:   effectiveRetentionLabel("audit_log.guild_retention_days", settings.AuditGuildRetentionDays),
	}
}

// retentionOverrideDisplay returns the value the override input should
// pre-fill. Stored 0 is rendered as "" when the ceiling is now finite —
// 0 was a legitimate "forever" override when the bot ceiling was also 0
// (legacy rows from before parseRetentionField started normalizing it),
// but if an operator has since lowered the ceiling the stored 0 is no
// longer valid per parseRetentionField. Rendering empty lets the user
// save again without first manually clearing the field.
func retentionOverrideDisplay(override *uint, ceiling uint) string {
	if override == nil {
		return ""
	}
	if *override == 0 && ceiling > 0 {
		return ""
	}
	return strconv.FormatUint(uint64(*override), 10)
}

func effectiveRetentionLabel(configKey string, override *uint) string {
	days, ok := scheduled_tasks.EffectiveRetentionDays(configKey, override)
	if !ok {
		return "kept forever"
	}
	return strconv.FormatUint(uint64(days), 10) + " days"
}

func uintFromConfig(key string) uint {
	v := viper.GetInt(key)
	if v < 0 {
		return 0
	}
	return uint(v)
}

func ptrUintToString(p *uint) string {
	if p == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*p), 10)
}

// handleSaveAuditLog persists the per-guild audit log toggle and retention
// overrides. Validates that overrides do not exceed the bot ceiling — sets
// to the ceiling instead of rejecting outright would be silently surprising,
// so we surface a save error with the offending field name.
func handleSaveAuditLog(client *bot.Client) http.HandlerFunc {
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
			// Surface config-derived ceilings even when the guild row is
			// unreadable, so the help text doesn't misleadingly imply
			// "kept forever" (ceiling=0) on every category.
			renderSafe(w, r, partials.SettingsAuditLog(partials.AuditLogSettingsData{
				GuildID:                 guildIDStr,
				SaveError:               "Failed to load settings.",
				MaxMessageRetentionDays: uintFromConfig("audit_log.message_retention_days"),
				MaxMemberRetentionDays:  uintFromConfig("audit_log.member_retention_days"),
				MaxGuildRetentionDays:   uintFromConfig("audit_log.guild_retention_days"),
			}))
			return
		}

		// Snapshot the user's submitted values so renderErr can echo them
		// back instead of resetting the form to the previously-saved row —
		// otherwise an invalid retention number wipes the user's typing
		// and forces them to start over.
		submittedEnabled := r.FormValue("enabled") == "true"
		submittedMessage := strings.TrimSpace(r.FormValue("message_retention_days"))
		submittedMember := strings.TrimSpace(r.FormValue("member_retention_days"))
		submittedGuild := strings.TrimSpace(r.FormValue("guild_retention_days"))

		renderErr := func(message string) {
			data := buildAuditLogSettingsData(guildIDStr, settings)
			data.Enabled = submittedEnabled
			data.MessageRetentionDaysOverride = submittedMessage
			data.MemberRetentionDaysOverride = submittedMember
			data.GuildRetentionDaysOverride = submittedGuild
			data.SaveError = message
			renderSafe(w, r, partials.SettingsAuditLog(data))
		}

		messageDays, err := parseRetentionField(submittedMessage, "audit_log.message_retention_days", "Message")
		if err != nil {
			renderErr(err.Error())
			return
		}
		memberDays, err := parseRetentionField(submittedMember, "audit_log.member_retention_days", "Member")
		if err != nil {
			renderErr(err.Error())
			return
		}
		guildDays, err := parseRetentionField(submittedGuild, "audit_log.guild_retention_days", "Guild")
		if err != nil {
			renderErr(err.Error())
			return
		}

		settings.AuditLogEnabled = submittedEnabled
		settings.AuditMessageRetentionDays = messageDays
		settings.AuditMemberRetentionDays = memberDays
		settings.AuditGuildRetentionDays = guildDays

		if err := model.SetGuildSettings(settings); err != nil {
			slog.Error("failed to save audit log settings", "error", err)
			renderErr("Failed to save settings.")
			return
		}
		// Clear the cached AuditLogEnabled flag so the new value takes
		// effect on the very next gateway event without waiting for TTL.
		audit.InvalidateShouldLogCache(guildID)
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "audit_log", map[string]any{
			"enabled":                settings.AuditLogEnabled,
			"message_retention_days": ptrUintToString(settings.AuditMessageRetentionDays),
			"member_retention_days":  ptrUintToString(settings.AuditMemberRetentionDays),
			"guild_retention_days":   ptrUintToString(settings.AuditGuildRetentionDays),
		})

		data := buildAuditLogSettingsData(guildIDStr, settings)
		data.SaveSuccess = true
		renderSafe(w, r, partials.SettingsAuditLog(data))
	}
}

// parseRetentionField turns "" into nil (= use default), a valid uint into
// a guild override, and rejects values above the bot ceiling. 0 is allowed
// only when the bot ceiling is also 0 (forever); even then it normalizes
// to nil since "override = forever" is indistinguishable from "no override
// when the ceiling is forever" — and persisting the explicit 0 would leave
// an invalid stored value if the operator later lowers the ceiling.
func parseRetentionField(raw, configKey, label string) (*uint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return nil, &settingsValidationError{label + " retention must be a non-negative whole number."}
	}
	v := uint(n)
	ceiling := uintFromConfig(configKey)
	if ceiling == 0 {
		if v == 0 {
			// "forever" with no ceiling is the same as no override —
			// collapse to nil so a future finite ceiling doesn't strand
			// this row with an invalid saved value.
			return nil, nil
		}
		return &v, nil
	}
	if v == 0 {
		return nil, &settingsValidationError{label + " retention may not be 0 (forever) — the bot ceiling is " + strconv.FormatUint(uint64(ceiling), 10) + " days."}
	}
	if v > ceiling {
		return nil, &settingsValidationError{label + " retention may not exceed the bot ceiling of " + strconv.FormatUint(uint64(ceiling), 10) + " days."}
	}
	return &v, nil
}

type settingsValidationError struct{ msg string }

func (e *settingsValidationError) Error() string { return e.msg }

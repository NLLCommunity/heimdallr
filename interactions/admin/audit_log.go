package admin

import (
	"fmt"
	"strconv"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/scheduled_tasks"
	"github.com/NLLCommunity/heimdallr/utils"
)

var auditLogSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "audit-log",
	Description: "View or set audit log settings",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionBool{
			Name:        "enabled",
			Description: "Whether to record audit log events for this guild",
			Required:    false,
		},
		discord.ApplicationCommandOptionInt{
			Name:        "message-retention",
			Description: "Override message-event retention in days. 0 = forever (only if bot ceiling is 0).",
			Required:    false,
			MinValue:    new(0),
		},
		discord.ApplicationCommandOptionInt{
			Name:        "member-retention",
			Description: "Override member-event retention in days. 0 = forever (only if bot ceiling is 0).",
			Required:    false,
			MinValue:    new(0),
		},
		discord.ApplicationCommandOptionInt{
			Name:        "guild-retention",
			Description: "Override guild-event retention in days. 0 = forever (only if bot ceiling is 0).",
			Required:    false,
			MinValue:    new(0),
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset a setting to use the bot-operator default",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Enabled", Value: "enabled"},
				{Name: "Message retention", Value: "message-retention"},
				{Name: "Member retention", Value: "member-retention"},
				{Name: "Guild retention", Value: "guild-retention"},
				{Name: "All", Value: "all"},
			},
		},
	},
}

func AdminAuditLogHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)

	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	resetOption, hasReset := data.OptString("reset")
	if hasReset {
		switch resetOption {
		case "enabled":
			settings.AuditLogEnabled = false
			message += "Audit log enabled has been reset.\n"
		case "message-retention":
			settings.AuditMessageRetentionDays = nil
			message += "Message retention override has been cleared.\n"
		case "member-retention":
			settings.AuditMemberRetentionDays = nil
			message += "Member retention override has been cleared.\n"
		case "guild-retention":
			settings.AuditGuildRetentionDays = nil
			message += "Guild retention override has been cleared.\n"
		case "all":
			settings.AuditLogEnabled = false
			settings.AuditMessageRetentionDays = nil
			settings.AuditMemberRetentionDays = nil
			settings.AuditGuildRetentionDays = nil
			message += "All audit log settings have been reset.\n"
		}
	}

	enabled, hasEnabled := data.OptBool("enabled")
	if hasEnabled {
		settings.AuditLogEnabled = enabled
		message += fmt.Sprintf("Audit log enabled set to %s\n", utils.Iif(enabled, "yes", "no"))
	}

	retentionOptions := []struct {
		optName   string
		configKey string
		label     string
		target    **uint
	}{
		{"message-retention", "audit_log.message_retention_days", "Message", &settings.AuditMessageRetentionDays},
		{"member-retention", "audit_log.member_retention_days", "Member", &settings.AuditMemberRetentionDays},
		{"guild-retention", "audit_log.guild_retention_days", "Guild", &settings.AuditGuildRetentionDays},
	}
	hasRetention := false
	for _, opt := range retentionOptions {
		msg, applied, err := applyRetentionOption(data, opt.optName, opt.configKey, opt.label, opt.target)
		if err != nil {
			return e.CreateMessage(interactions.EphemeralMessageContent(err.Error()))
		}
		if applied {
			message += msg
			hasRetention = true
		}
	}

	if !utils.Any(hasEnabled, hasRetention, hasReset) {
		return e.CreateMessage(interactions.EphemeralMessageContent(auditLogInfo(settings)))
	}

	if err := model.SetGuildSettings(settings); err != nil {
		return err
	}
	// Clear the cached AuditLogEnabled flag so the new value takes effect
	// on the very next gateway event without waiting for TTL — mirrors the
	// dashboard handler's invalidate.
	audit.InvalidateShouldLogCache(guild.ID)
	logSettingsCommandUpdate(guild.ID, e.User(), "audit_log", map[string]any{
		"enabled":                settings.AuditLogEnabled,
		"message_retention_days": uintPtrString(settings.AuditMessageRetentionDays),
		"member_retention_days":  uintPtrString(settings.AuditMemberRetentionDays),
		"guild_retention_days":   uintPtrString(settings.AuditGuildRetentionDays),
	})

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

// applyRetentionOption reads a retention option from the command data,
// validates it via the shared scheduled_tasks rules (the same ones the
// dashboard's parseRetentionField uses, so the two write paths cannot
// drift apart), and writes the resulting *uint into target. Returns
// (statusLine, applied, validationError).
func applyRetentionOption(
	data discord.SlashCommandInteractionData,
	optName, configKey, label string,
	target **uint,
) (string, bool, error) {
	raw, ok := data.OptInt(optName)
	if !ok {
		return "", false, nil
	}
	if raw < 0 {
		// Defensive: MinValue: 0 on the option already blocks this at
		// Discord's edge, but a malicious or buggy client could still
		// send a negative.
		return "", false, fmt.Errorf("%s retention must be a non-negative whole number", label)
	}
	v := uint(raw)
	override, err := scheduled_tasks.ValidateRetentionOverride(configKey, label, v)
	if err != nil {
		return "", false, err
	}
	*target = override
	if override == nil {
		// "forever" with no ceiling collapses to nil, same as the
		// dashboard. Keeps stored rows portable across ceiling changes.
		return fmt.Sprintf("%s retention override cleared (was 0 with no ceiling).\n", label), true, nil
	}
	return fmt.Sprintf("%s retention override set to %d days.\n", label, v), true, nil
}

func auditLogInfo(settings *model.GuildSettings) string {
	enabledInfo := "> When disabled, no events are recorded for this guild. Disabling does not prune existing rows."
	enabled := fmt.Sprintf(
		"**Audit log enabled:** %s\n%s",
		utils.Iif(settings.AuditLogEnabled, "yes", "no"), enabledInfo,
	)

	return fmt.Sprintf(
		"## Audit log settings\n%s\n\n%s\n\n%s\n\n%s",
		enabled,
		retentionLine("Message", "audit_log.message_retention_days", settings.AuditMessageRetentionDays),
		retentionLine("Member", "audit_log.member_retention_days", settings.AuditMemberRetentionDays),
		retentionLine("Guild", "audit_log.guild_retention_days", settings.AuditGuildRetentionDays),
	)
}

// retentionLine renders one retention row with both the configured
// override (or "default") and the effective window the pruner will use.
// The pruner is what actually drives behaviour, so showing the resolved
// value matters for the "did my change take effect?" question.
func retentionLine(label, configKey string, override *uint) string {
	var overrideStr string
	if override == nil {
		overrideStr = "default"
	} else {
		overrideStr = strconv.FormatUint(uint64(*override), 10) + " days"
	}

	days, ok := scheduled_tasks.EffectiveRetentionDays(configKey, override)
	var effective string
	if !ok {
		effective = "kept forever"
	} else {
		effective = strconv.FormatUint(uint64(days), 10) + " days"
	}

	return fmt.Sprintf("**%s retention:** override=%s, effective=%s", label, overrideStr, effective)
}

// uintPtrString renders *uint as either the decimal value or "" so the
// audit-log entry detail map matches what the dashboard logs (allowing
// the audit viewer to render uniformly regardless of where the change
// originated).
func uintPtrString(p *uint) string {
	if p == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*p), 10)
}

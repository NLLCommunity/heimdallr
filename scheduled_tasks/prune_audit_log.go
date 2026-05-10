package scheduled_tasks

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/spf13/viper"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/task"
)

// PruneAuditLogScheduledTask deletes audit log entries past their per-guild
// effective retention window. Effective retention = min(bot ceiling,
// guild override) — guilds can only lower retention versus the bot config,
// not raise it.
//
// 0 (in either bot config or guild override) means "forever"; the pruner
// skips those (guild, category) pairs entirely.
func PruneAuditLogScheduledTask() task.Task {
	interval := time.Duration(viper.GetInt("audit_log.prune_interval_hours")) * time.Hour
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	t := task.New("prune-audit-log", pruneAuditLog, nil, interval)
	t.StartNoWait()
	return t
}

func pruneAuditLog(ctx context.Context) {
	guildIDs, err := model.DistinctAuditLogGuilds()
	if err != nil {
		slog.Warn("audit pruner: failed to list guilds", "err", err)
		return
	}

	totals := map[string]int64{
		string(audit.CategoryMessage): 0,
		string(audit.CategoryMember):  0,
		string(audit.CategoryGuild):   0,
	}

	for _, guildID := range guildIDs {
		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			slog.Warn("audit pruner: failed to read guild settings", "err", err, "guild_id", guildID)
			continue
		}

		for _, c := range []categoryRetention{
			{audit.CategoryMessage, "audit_log.message_retention_days", settings.AuditMessageRetentionDays},
			{audit.CategoryMember, "audit_log.member_retention_days", settings.AuditMemberRetentionDays},
			{audit.CategoryGuild, "audit_log.guild_retention_days", settings.AuditGuildRetentionDays},
		} {
			deleted, err := pruneCategory(ctx, guildID, c)
			if err != nil {
				slog.Warn("audit pruner: prune failed",
					"err", err, "guild_id", guildID, "category", c.category)
				continue
			}
			totals[string(c.category)] += deleted
		}
	}

	slog.Info("audit pruner finished",
		"guilds", len(guildIDs),
		"deleted_message", totals[string(audit.CategoryMessage)],
		"deleted_member", totals[string(audit.CategoryMember)],
		"deleted_guild", totals[string(audit.CategoryGuild)],
	)
}

type categoryRetention struct {
	category   audit.Category
	configKey  string
	guildValue *uint
}

func pruneCategory(ctx context.Context, guildID snowflake.ID, c categoryRetention) (int64, error) {
	days, ok := EffectiveRetentionDays(c.configKey, c.guildValue)
	if !ok {
		// Forever: do not prune.
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	return model.PruneAuditLogEntriesBefore(ctx, guildID, string(c.category), cutoff)
}

// EffectiveRetentionDays resolves the effective retention window in days for
// a given category. Returns (days, true) when there's a finite window, or
// (0, false) when retention is "forever" (either ceiling or override is 0,
// which the settings handler permits only when the ceiling is 0).
//
// Exposed so the settings web handler can validate guild overrides against
// the bot ceiling without duplicating the resolution logic.
func EffectiveRetentionDays(configKey string, guildOverride *uint) (uint, bool) {
	ceiling := viper.GetInt(configKey)
	if ceiling < 0 {
		ceiling = 0
	}

	if guildOverride == nil {
		if ceiling == 0 {
			return 0, false
		}
		return uint(ceiling), true
	}

	override := *guildOverride
	if ceiling == 0 {
		// Bot says forever; guild may opt for any finite window.
		if override == 0 {
			return 0, false
		}
		return override, true
	}

	// Bot has a ceiling. Guild override caps at the ceiling; 0 disallowed.
	if override == 0 || override > uint(ceiling) {
		return uint(ceiling), true
	}
	return override, true
}

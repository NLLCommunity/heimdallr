package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

// AuditLogEntry is one row in the bot-owned audit log. Rows are written by
// gateway listeners, slash command handlers, and web dashboard handlers via
// the audit package; queries for the viewer page live in this file.
//
// Rows are immutable once committed and pruned by the scheduled retention
// task — there's no soft-delete, no UpdatedAt.
type AuditLogEntry struct {
	ID        uint         `gorm:"primaryKey"`
	GuildID   snowflake.ID `gorm:"index:idx_audit_guild_created;index:idx_audit_guild_actor;index:idx_audit_guild_target;index:idx_audit_guild_event;not null"`
	CreatedAt time.Time    `gorm:"index:idx_audit_guild_created,sort:desc;autoCreateTime"`

	Category  string `gorm:"not null;index"`
	EventType string `gorm:"not null;index:idx_audit_guild_event"`

	ActorID    *snowflake.ID `gorm:"index:idx_audit_guild_actor"`
	ActorKind  string        `gorm:"not null"`
	TargetID   *snowflake.ID `gorm:"index:idx_audit_guild_target"`
	TargetKind string        `gorm:"not null"`

	Source string `gorm:"not null;index"`
	Reason string
	// Details is the JSON-encoded event-specific payload. We store as text
	// rather than using a typed JSON column to match the existing pattern
	// in model.Post (ComponentsJSON) and avoid a new dependency.
	Details string `gorm:"type:text"`
}

// AuditLogFilter narrows AuditLogEntry queries. Zero-valued fields are
// ignored; only set the axes the caller wants to filter on.
//
// Actor and target each have three matching axes that are OR'd together
// so a single query string can hit any of them:
//
//   - ActorIDs / TargetIDs: exact actor_id / target_id match. Populated
//     from cached member / channel name lookups in the web layer.
//   - ActorQuery / TargetQuery: case-insensitive substring match against
//     details.actor_username / details.target_username. Used as a fallback
//     when the disgo cache is empty (e.g. after a bot restart).
//   - TargetChannelIDs: matches details.channel_id, used so a search like
//     "#general" finds message events that occurred in that channel even
//     though their target_id is the message ID, not the channel.
//
// From is inclusive, To is exclusive — matching the convention that an
// empty To means "now or later" while an empty From means "any time".
type AuditLogFilter struct {
	Category         string
	EventType        string
	ActorIDs         []snowflake.ID
	ActorQuery       string
	TargetIDs        []snowflake.ID
	TargetChannelIDs []snowflake.ID
	TargetQuery      string
	Source           string
	From             time.Time
	To               time.Time
}

// applyTo mutates a *gorm.DB with the filter's WHERE clauses. Caller is
// responsible for setting the guild scope before calling this.
func (f AuditLogFilter) applyTo(tx *gorm.DB) *gorm.DB {
	if f.Category != "" {
		tx = tx.Where("category = ?", f.Category)
	}
	if f.EventType != "" {
		tx = tx.Where("event_type = ?", f.EventType)
	}
	if clause, args := buildPersonClause("actor_id", "actor_username", f.ActorIDs, nil, f.ActorQuery); clause != "" {
		tx = tx.Where(clause, args...)
	}
	if clause, args := buildPersonClause("target_id", "target_username", f.TargetIDs, f.TargetChannelIDs, f.TargetQuery); clause != "" {
		tx = tx.Where(clause, args...)
	}
	if f.Source != "" {
		tx = tx.Where("source = ?", f.Source)
	}
	if !f.From.IsZero() {
		tx = tx.Where("created_at >= ?", f.From)
	}
	if !f.To.IsZero() {
		tx = tx.Where("created_at < ?", f.To)
	}
	return tx
}

// buildPersonClause assembles an OR clause matching an entry by direct
// snowflake column, by channel_id stored in the JSON details (for
// message events whose target is the message itself), or by a
// case-insensitive substring of the username field stored in details.
//
// Returns ("", nil) when none of the inputs are populated so the caller
// can skip adding any WHERE clause.
//
// idCol and jsonField are internal constants ("actor_id" / "target_id"
// and "actor_username" / "target_username"); not user-controlled, so
// concatenating them into the SQL fragment is safe. All variable values
// flow through ? placeholders.
//
// Every json_extract is gated by json_valid(details) so rows with empty
// or malformed JSON (legacy rows, tests that bypass audit.commit) don't
// error the query; SQLite's AND short-circuits the json_extract call
// when json_valid returns 0. Same predicate as idx_audit_details_channel
// so the planner can use the partial expression index for channel
// filters.
func buildPersonClause(idCol, jsonField string, ids, channelIDs []snowflake.ID, query string) (string, []any) {
	var parts []string
	var args []any

	if len(ids) > 0 {
		parts = append(parts, idCol+" IN "+placeholderList(len(ids)))
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(channelIDs) > 0 {
		// channel_id is stored as a string in the JSON payload, so we
		// match against stringified IDs to keep type coercion explicit.
		parts = append(parts,
			"(json_valid(details) AND json_extract(details, '$.channel_id') IN "+placeholderList(len(channelIDs))+")")
		for _, id := range channelIDs {
			args = append(args, id.String())
		}
	}
	if query != "" {
		parts = append(parts,
			"(json_valid(details) AND LOWER(json_extract(details, '$."+jsonField+"')) LIKE ?)")
		args = append(args, "%"+strings.ToLower(query)+"%")
	}

	if len(parts) == 0 {
		return "", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

func placeholderList(n int) string {
	if n <= 0 {
		return "()"
	}
	return "(" + strings.Repeat("?,", n-1) + "?)"
}

// ListAuditLogEntries returns a slice of entries matching the filter, sorted
// newest-first, plus the unpaginated total for the same filter (so the UI
// can render "page X of Y"). Mirrors the shape of GetUserInfractions.
func ListAuditLogEntries(guildID snowflake.ID, filter AuditLogFilter, limit, offset int) ([]AuditLogEntry, int64, error) {
	var entries []AuditLogEntry
	q := DB.Where("guild_id = ?", guildID)
	q = filter.applyTo(q)
	res := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&entries)
	if res.Error != nil {
		return nil, 0, res.Error
	}

	var count int64
	cq := DB.Model(&AuditLogEntry{}).Where("guild_id = ?", guildID)
	cq = filter.applyTo(cq)
	if err := cq.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	return entries, count, nil
}

// GetAuditLogEntry fetches a single entry by ID, scoped to a guild so a
// caller from one guild can't view another's rows even if they guess an ID.
func GetAuditLogEntry(guildID snowflake.ID, id uint) (*AuditLogEntry, error) {
	var entry AuditLogEntry
	res := DB.Where("guild_id = ? AND id = ?", guildID, id).First(&entry)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, res.Error
		}
		return nil, res.Error
	}
	return &entry, nil
}

// PruneAuditLogEntriesBefore deletes entries in the given guild and category
// whose CreatedAt is older than cutoff. Returns the number of rows deleted.
//
// Used by the retention scheduled task. Pass a zero cutoff to no-op (matching
// the "0 = forever" config convention).
func PruneAuditLogEntriesBefore(ctx context.Context, guildID snowflake.ID, category string, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, nil
	}
	res := DB.WithContext(ctx).
		Where("guild_id = ? AND category = ? AND created_at < ?", guildID, category, cutoff).
		Delete(&AuditLogEntry{})
	return res.RowsAffected, res.Error
}

// DistinctAuditLogGuilds returns every guild ID with at least one audit log
// row. Used by the pruner to avoid scanning every GuildSettings row when
// most guilds have audit logging disabled.
func DistinctAuditLogGuilds() ([]snowflake.ID, error) {
	var ids []snowflake.ID
	res := DB.Model(&AuditLogEntry{}).Distinct("guild_id").Pluck("guild_id", &ids)
	return ids, res.Error
}

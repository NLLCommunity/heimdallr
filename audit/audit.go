// Package audit owns the bot's audit log: a record of moderation-relevant
// actions across gateway events, bot commands, and web dashboard mutations.
//
// Callers use Log for events with full information at the call site, and
// LogPending for gateway events whose actor or reason may be supplied later
// by Discord's native audit log (see pending.go).
//
// Persistence lives in model.AuditLogEntry. Category, EventType, and the
// other classifiers are plain string types so the model layer can stay
// gorm-only.
package audit

import (
	"encoding/json"
	"log/slog"

	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
)

// Category groups audit log entries for retention purposes. Each category has
// its own retention window in the bot config and may be overridden per guild.
type Category string

const (
	CategoryMessage Category = "message"
	CategoryMember  Category = "member"
	CategoryGuild   Category = "guild"
)

// EventType identifies the specific audit log event. Format: "<area>.<verb>".
type EventType string

const (
	EventMessageEdit   EventType = "message.edit"
	EventMessageDelete EventType = "message.delete"

	EventMemberJoin EventType = "member.join"
	EventMemberLeave  EventType = "member.leave"
	// EventMemberUpdate is retained for filter compatibility with rows
	// written before the split into per-change types below. New entries
	// use the more specific event types so the viewer can filter on
	// "Member timed out" vs "Roles changed" individually.
	EventMemberUpdate       EventType = "member.update"
	EventMemberNickChange   EventType = "member.nick_change"
	EventMemberRoleChange   EventType = "member.role_change"
	EventMemberTimeoutAdd   EventType = "member.timeout_add"
	EventMemberTimeoutClear EventType = "member.timeout_clear"

	EventGuildBan   EventType = "guild.ban"
	EventGuildUnban EventType = "guild.unban"
	EventGuildKick  EventType = "guild.kick"

	EventBotWarn       EventType = "bot.warn"
	EventBotInfraction EventType = "bot.infraction"
	EventBotTimeout    EventType = "bot.timeout"

	EventWebSettingsUpdate EventType = "web.settings.update"
	EventWebPostCreate     EventType = "web.post.create"
	EventWebPostUpdate     EventType = "web.post.update"
	EventWebPostDelete     EventType = "web.post.delete"
)

// ActorKind disambiguates the namespace of ActorID — same numeric snowflake
// ID space spans users, bots, etc. "system" is used for bot-internal actions
// without a clear actor; "unknown" is used when we know an actor exists but
// can't determine who it was (e.g. moderator message-delete with no native
// audit log info available).
type ActorKind string

const (
	ActorUser    ActorKind = "user"
	ActorBot     ActorKind = "bot"
	ActorSystem  ActorKind = "system"
	ActorUnknown ActorKind = "unknown"
)

// TargetKind disambiguates the namespace of TargetID.
type TargetKind string

const (
	TargetUser    TargetKind = "user"
	TargetChannel TargetKind = "channel"
	TargetRole    TargetKind = "role"
	TargetMessage TargetKind = "message"
	TargetGuild   TargetKind = "guild"
	TargetNone    TargetKind = "none"
)

// Source identifies the path that produced the entry, useful both for
// diagnostics and as a UI filter axis.
type Source string

const (
	SourceGateway Source = "gateway"
	SourceCommand Source = "command"
	SourceWeb     Source = "web"
)

// EventCategory maps an EventType to its retention Category. Returns ""
// for unknown types — callers should fail closed and skip those.
func EventCategory(t EventType) Category {
	switch t {
	case EventMessageEdit, EventMessageDelete:
		return CategoryMessage
	case EventMemberJoin, EventMemberLeave, EventMemberUpdate,
		EventMemberNickChange, EventMemberRoleChange,
		EventMemberTimeoutAdd, EventMemberTimeoutClear:
		return CategoryMember
	case EventGuildBan, EventGuildUnban, EventGuildKick,
		EventBotWarn, EventBotInfraction, EventBotTimeout,
		EventWebSettingsUpdate, EventWebPostCreate, EventWebPostUpdate, EventWebPostDelete:
		return CategoryGuild
	}
	return ""
}

// Entry is the in-flight representation passed to Log / LogPending. It maps
// onto model.AuditLogEntry for persistence; using a separate struct keeps
// callers from having to import gorm or worry about gorm tags.
//
// Category is derived from EventType via EventCategory if left empty.
type Entry struct {
	GuildID    snowflake.ID
	Category   Category
	EventType  EventType
	ActorID    *snowflake.ID
	ActorKind  ActorKind
	TargetID   *snowflake.ID
	TargetKind TargetKind
	Source     Source
	Reason     string
	Details    map[string]any
}

// Log writes the entry immediately. Use this for events that need no
// enrichment from Discord's native audit log — member join/leave, bot
// command actions, web dashboard actions.
//
// Failures are logged at warn but never returned: an audit-log write
// failure must not break the calling moderation flow.
func Log(entry Entry) {
	if entry.Category == "" {
		entry.Category = EventCategory(entry.EventType)
	}
	if entry.Category == "" {
		slog.Warn("audit: dropping entry with unknown event type", "event", entry.EventType)
		return
	}
	if !shouldLog(entry.GuildID) {
		return
	}
	commit(entry)
}

// shouldLog checks the per-guild master toggle. Returns false on settings
// read errors — failing closed is safer than logging when we can't confirm
// the guild has opted in.
func shouldLog(guildID snowflake.ID) bool {
	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		slog.Warn("audit: failed to read guild settings", "err", err, "guild_id", guildID)
		return false
	}
	return settings.AuditLogEnabled
}

// commit serializes details and writes the row. Errors are logged at warn
// rather than returned for the same fail-soft reason as Log.
func commit(entry Entry) {
	detailsJSON, err := marshalDetails(entry.Details)
	if err != nil {
		slog.Warn("audit: failed to marshal details", "err", err, "event", entry.EventType)
		detailsJSON = "{}"
	}
	row := &model.AuditLogEntry{
		GuildID:    entry.GuildID,
		Category:   string(entry.Category),
		EventType:  string(entry.EventType),
		ActorID:    entry.ActorID,
		ActorKind:  string(entry.ActorKind),
		TargetID:   entry.TargetID,
		TargetKind: string(entry.TargetKind),
		Source:     string(entry.Source),
		Reason:     entry.Reason,
		Details:    detailsJSON,
	}
	if err := model.DB.Create(row).Error; err != nil {
		slog.Warn("audit: failed to write entry",
			"err", err,
			"event", entry.EventType,
			"guild_id", entry.GuildID,
		)
	}
}

func marshalDetails(d map[string]any) (string, error) {
	if len(d) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SnowflakePtr is a small helper since callers frequently need to take the
// address of a snowflake.ID literal for the Entry pointer fields.
func SnowflakePtr(id snowflake.ID) *snowflake.ID {
	return &id
}

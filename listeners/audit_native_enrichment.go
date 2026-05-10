package listeners

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/audit"
)

// OnAuditNativeEnrichment is a second listener for GuildAuditLogEntryCreate
// dedicated to the audit log feature. It either:
//
//   - enriches a pending audit log entry (committed by gateway listeners
//     via audit.LogPending) with moderator attribution; or
//   - writes a fresh row for events Discord only reports through the
//     native audit log (member kicks, prune-driven removals).
//
// The existing OnAuditLogKick listener (in audit_log_kick.go) is left
// untouched — it owns the moderator-channel kick notification flow.
// disgo allows multiple listeners on the same event, so the two coexist.
func OnAuditNativeEnrichment(e *events.GuildAuditLogEntryCreate) {
	entry := e.AuditLogEntry
	guildID := e.GuildID

	actorID := entry.UserID
	actorPtr := &actorID
	// Resolve the moderator's username via the disgo cache, falling back
	// to a REST fetch on miss so the viewer renders @username instead of
	// the raw snowflake when the cache is cold. Latency cost is a single
	// network round-trip on cache miss, paid once per audit log entry.
	actorUsername := audit.ResolveMemberUsernameOrFetch(e.Client(), guildID, actorID)
	reason := ""
	if entry.Reason != nil {
		reason = *entry.Reason
	}

	switch entry.ActionType {
	case discord.AuditLogEventMessageDelete, discord.AuditLogEventMessageBulkDelete:
		// Bulk and single both enrich every pending message-delete in the
		// guild within the TTL window. Native audit doesn't tell us which
		// individual messages a bulk delete covered, so the wildcard match
		// (nil targetID) is the best available scope.
		audit.TryEnrich(guildID, audit.EventMessageDelete, nil, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchAll)

	case discord.AuditLogEventMemberBanAdd:
		var target *snowflake.ID
		if entry.TargetID != nil {
			id := *entry.TargetID
			target = &id
		}
		audit.TryEnrich(guildID, audit.EventGuildBan, target, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchFirst)

	case discord.AuditLogEventMemberBanRemove:
		var target *snowflake.ID
		if entry.TargetID != nil {
			id := *entry.TargetID
			target = &id
		}
		audit.TryEnrich(guildID, audit.EventGuildUnban, target, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchFirst)

	case discord.AuditLogEventMemberUpdate, discord.AuditLogEventMemberRoleUpdate:
		// MemberUpdate / MemberRoleUpdate from native audit only fire when
		// SOMEONE ELSE changed the target. Self-updates (own nickname) get
		// no native audit entry, which is the right behaviour: the pending
		// entry just commits unenriched.
		//
		// We split by the change key so the right pending event type
		// gets enriched: timeout / nick / role changes are separate
		// audit log event types since this last refactor.
		var target *snowflake.ID
		if entry.TargetID != nil {
			id := *entry.TargetID
			target = &id
		}
		for _, ev := range memberUpdateEnrichmentTargets(entry.Changes) {
			audit.TryEnrich(guildID, ev, target, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchFirst)
		}

	case discord.AuditLogEventMemberKick, discord.AuditLogEventMemberPrune:
		if entry.TargetID == nil && entry.ActionType == discord.AuditLogEventMemberKick {
			// Kick should always have a target; bail rather than write
			// a useless row.
			return
		}
		// A kick fires both a GuildMemberLeave (from gateway) and this
		// native entry. Drop the pending leave so we don't record both
		// rows for the same departure. Prune is treated the same way:
		// disgo emits a leave for each pruned member.
		var target *snowflake.ID
		if entry.TargetID != nil {
			id := *entry.TargetID
			target = &id
			audit.CancelPending(guildID, audit.EventMemberLeave, target)
		}

		details := map[string]any{}
		if entry.ActionType == discord.AuditLogEventMemberPrune && entry.Options != nil {
			if entry.Options.MembersRemoved != nil && *entry.Options.MembersRemoved != "" {
				details["members_removed"] = *entry.Options.MembersRemoved
			}
			if entry.Options.DeleteMemberDays != nil && *entry.Options.DeleteMemberDays != "" {
				details["delete_member_days"] = *entry.Options.DeleteMemberDays
			}
		}
		// Capture usernames at write time. Use the REST-fallback resolver
		// so a kicked member who's just been removed from the cache still
		// resolves — the user-visible audit log row is the only place
		// these names will surface, and a REST round-trip per kick is
		// cheap enough (kicks are rare).
		if actorUsername != "" {
			details["actor_username"] = actorUsername
		}
		if target != nil {
			if name := audit.ResolveMemberUsernameOrFetch(e.Client(), guildID, *target); name != "" {
				details["target_username"] = name
			}
		}

		audit.Log(audit.Entry{
			GuildID:    guildID,
			EventType:  audit.EventGuildKick,
			ActorID:    actorPtr,
			ActorKind:  audit.ActorUser,
			TargetID:   target,
			TargetKind: audit.TargetUser,
			Source:     audit.SourceGateway,
			Reason:     reason,
			Details:    details,
		})
	}
}

// memberUpdateEnrichmentTargets inspects a Discord audit log entry's
// Changes array and returns the audit log event types that should be
// enriched. A single MemberUpdate native entry can carry multiple changes
// (e.g. mod sets nick AND adds timeout in one click), so this returns
// a slice rather than picking one.
//
// Returns the legacy EventMemberUpdate as a fallback when none of the
// known keys are present, so unrecognised member changes still get an
// actor where possible without us having to enumerate every Discord
// audit-log change key.
func memberUpdateEnrichmentTargets(changes []discord.AuditLogChange) []audit.EventType {
	var out []audit.EventType
	for _, c := range changes {
		switch c.Key {
		case discord.AuditLogChangeKeyCommunicationDisabledUntil:
			// Distinguish add vs clear by inspecting the new value. A
			// JSON null new_value means the timeout was lifted.
			if isNullJSON(c.NewValue) {
				out = append(out, audit.EventMemberTimeoutClear)
			} else {
				out = append(out, audit.EventMemberTimeoutAdd)
			}
		case discord.AuditLogChangeKeyNick:
			out = append(out, audit.EventMemberNickChange)
		case discord.AuditLogChangeKeyRoleAdd, discord.AuditLogChangeKeyRoleRemove:
			out = append(out, audit.EventMemberRoleChange)
		}
	}
	if len(out) == 0 {
		out = append(out, audit.EventMemberUpdate)
	}
	return out
}

// isNullJSON returns true when raw is the JSON literal null or empty.
// Used to tell a cleared timeout from one being set.
func isNullJSON(raw []byte) bool {
	s := string(raw)
	return s == "" || s == "null"
}

package listeners

import (
	"strconv"

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
// Scope is deliberately limited to moderation-relevant action types:
// message delete/bulk-delete, ban/unban, member update (timeout, nick,
// role), kick, and prune. Discord's native audit log also reports voice
// channel moves/disconnects, channel/role/webhook create/delete/update,
// and similar — these are intentionally not surfaced; the audit log
// feature targets moderation activity, not full server-config history.
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
	// to a REST GetMember on miss.
	actorUsername := audit.ResolveMemberUsernameOrFetch(e.Client(), guildID, actorID)
	reason := ""
	if entry.Reason != nil {
		reason = *entry.Reason
	}

	switch entry.ActionType {
	case discord.AuditLogEventMessageDelete:
		// Single (sometimes Discord-aggregated) message-delete entry.
		// Native side reports TargetID = author and Options.ChannelID =
		// channel; pending side stored those same values in Details. Match
		// on (channel_id, author_id) so concurrent unrelated deletes —
		// e.g. a user self-deleting in another channel — aren't swept
		// into the moderator's attribution. Cache-miss pendings (no
		// author_id captured) deliberately fail to match and commit
		// unenriched rather than risk misattribution.
		//
		// Discord aggregates repeated deletes of one author's messages
		// in one channel into a single native entry with Count > 1, so
		// MatchAll is the right mode when count > 1, capped at exactly
		// Count consumptions so the buffered enrichment can't latch
		// onto an unrelated same-(channel, author) delete arriving
		// later within TTL. Count == 1 stays MatchFirst (one-shot).
		if entry.TargetID == nil || entry.Options == nil || entry.Options.ChannelID == nil {
			return
		}
		count := auditEntryCount(entry.Options.Count)
		match := audit.MatchFirst
		maxMatches := 0
		if count > 1 {
			match = audit.MatchAll
			maxMatches = count
		}
		required := map[string]string{
			"channel_id": entry.Options.ChannelID.String(),
			"author_id":  entry.TargetID.String(),
		}
		audit.TryEnrich(guildID, audit.EventMessageDelete, nil, required, actorPtr, audit.ActorUser, actorUsername, reason, match, maxMatches)

	case discord.AuditLogEventMessageBulkDelete:
		// Bulk-delete entry's TargetID IS the channel (Options carries
		// the count but isn't required for matching). Sweep every
		// pending message-delete whose Details.channel_id matches,
		// capped by Options.Count when known so the enrichment expires
		// once the burst is fully attributed.
		//
		// Fall back to Discord's documented bulk-delete API ceiling
		// (100 messages per request) when Options/Count is missing —
		// keeps the misattribution window finite if Discord ever omits
		// the count, rather than leaving an unlimited sticky enrichment
		// that any same-channel self-delete within TTL could latch onto.
		if entry.TargetID == nil {
			return
		}
		maxMatches := bulkDeleteCeiling
		if entry.Options != nil {
			if c := auditEntryCount(entry.Options.Count); c > 0 {
				maxMatches = c
			}
		}
		required := map[string]string{
			"channel_id": entry.TargetID.String(),
		}
		audit.TryEnrich(guildID, audit.EventMessageDelete, nil, required, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchAll, maxMatches)

	case discord.AuditLogEventMemberBanAdd:
		var target *snowflake.ID
		if entry.TargetID != nil {
			id := *entry.TargetID
			target = &id
		}
		audit.TryEnrich(guildID, audit.EventGuildBan, target, nil, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchFirst, 0)

	case discord.AuditLogEventMemberBanRemove:
		var target *snowflake.ID
		if entry.TargetID != nil {
			id := *entry.TargetID
			target = &id
		}
		audit.TryEnrich(guildID, audit.EventGuildUnban, target, nil, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchFirst, 0)

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
			audit.TryEnrich(guildID, ev, target, nil, actorPtr, audit.ActorUser, actorUsername, reason, audit.MatchFirst, 0)
		}

	case discord.AuditLogEventMemberKick:
		if entry.TargetID == nil {
			// Kick should always have a target; bail rather than write
			// a useless row.
			return
		}
		id := *entry.TargetID
		target := &id

		details := map[string]any{}
		// Capture usernames at write time. The target is by definition no
		// longer a guild member at this point, so GetMember would 404;
		// use the user-level resolver that falls back to Rest.GetUser.
		// One REST round-trip per kick is cheap — kicks are rare.
		if actorUsername != "" {
			details["actor_username"] = actorUsername
		}
		if name := audit.ResolveUserUsernameOrFetch(e.Client(), guildID, *target); name != "" {
			details["target_username"] = name
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

	case discord.AuditLogEventMemberPrune:
		// Prune fires one native audit log entry for the whole batch
		// (TargetID is nil) plus per-member gateway leave events. The
		// gateway leaves are not logged — the audit log only records the
		// single guild.prune row here, capturing the moderator and
		// batch metadata.
		details := map[string]any{}
		if entry.Options != nil {
			if entry.Options.MembersRemoved != nil && *entry.Options.MembersRemoved != "" {
				details["members_removed"] = *entry.Options.MembersRemoved
			}
			if entry.Options.DeleteMemberDays != nil && *entry.Options.DeleteMemberDays != "" {
				details["delete_member_days"] = *entry.Options.DeleteMemberDays
			}
		}
		if actorUsername != "" {
			details["actor_username"] = actorUsername
		}

		audit.Log(audit.Entry{
			GuildID:    guildID,
			EventType:  audit.EventGuildPrune,
			ActorID:    actorPtr,
			ActorKind:  audit.ActorUser,
			TargetKind: audit.TargetNone,
			Source:     audit.SourceGateway,
			Reason:     reason,
			Details:    details,
		})
	}
}

// memberUpdateEnrichmentTargets inspects a Discord audit log entry's
// Changes array and returns the distinct audit log event types that
// should be enriched. A single MemberUpdate native entry can carry
// multiple changes (e.g. mod sets nick AND adds timeout in one click),
// so this returns a slice rather than picking one.
//
// Duplicates are filtered: a native entry that adds AND removes roles in
// one operation only enriches EventMemberRoleChange once. Without this,
// the second TryEnrich(MatchFirst) call would find no pending entry and
// buffer a stray enrichment that could misattribute the next unrelated
// gateway event for the same key within pendingTTL.
//
// Returns the legacy EventMemberUpdate as a fallback when none of the
// known keys are present, so unrecognised member changes still get an
// actor where possible without us having to enumerate every Discord
// audit-log change key.
func memberUpdateEnrichmentTargets(changes []discord.AuditLogChange) []audit.EventType {
	var out []audit.EventType
	seen := map[audit.EventType]bool{}
	add := func(ev audit.EventType) {
		if seen[ev] {
			return
		}
		seen[ev] = true
		out = append(out, ev)
	}
	for _, c := range changes {
		switch c.Key {
		case discord.AuditLogChangeKeyCommunicationDisabledUntil:
			// Distinguish add vs clear by inspecting the new value. A
			// JSON null new_value means the timeout was lifted.
			if isNullJSON(c.NewValue) {
				add(audit.EventMemberTimeoutClear)
			} else {
				add(audit.EventMemberTimeoutAdd)
			}
		case discord.AuditLogChangeKeyNick:
			add(audit.EventMemberNickChange)
		case discord.AuditLogChangeKeyRoleAdd, discord.AuditLogChangeKeyRoleRemove:
			add(audit.EventMemberRoleChange)
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

// bulkDeleteCeiling is Discord's documented per-request cap on bulk
// message deletes (POST .../messages/bulk-delete accepts at most 100
// IDs). Used as the cap on bulk-delete enrichment when Options.Count
// is missing so the buffered enrichment can't latch onto unlimited
// late-arriving pendings.
const bulkDeleteCeiling = 100

// auditEntryCount parses Discord's stringly-typed Options.Count field.
// Returns 0 when nil or unparseable so callers can branch on "unknown".
func auditEntryCount(raw *string) int {
	if raw == nil || *raw == "" {
		return 0
	}
	n, err := strconv.Atoi(*raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

package listeners

import (
	"slices"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/utils"
)

// OnAuditMemberUpdate writes one audit log entry per kind of change in a
// GuildMemberUpdate event — timeout add/clear, role change, nick change.
// Splitting these into distinct event types lets the viewer filter on
// "Member timed out" or "Roles changed" specifically rather than treating
// them all as one bucket.
//
// Each entry routes through LogPending so the actor (a moderator other
// than the member themselves) can be filled in by the native audit log
// enrichment listener.
func OnAuditMemberUpdate(e *events.GuildMemberUpdate) {
	target := e.Member.User.ID
	old := e.OldMember
	updated := e.Member
	username := updated.User.Username

	emit := func(eventType audit.EventType, details map[string]any) {
		details["target_username"] = username
		audit.LogPending(audit.Entry{
			GuildID:    e.GuildID,
			EventType:  eventType,
			ActorKind:  audit.ActorUnknown, // overwritten by enrichment when available
			TargetID:   &target,
			TargetKind: audit.TargetUser,
			Source:     audit.SourceGateway,
			Details:    details,
		}, []audit.EnrichField{audit.EnrichActor, audit.EnrichReason})
	}

	// Disgo populates OldMember from the cache before applying the new
	// state. If the member wasn't cached (common after a bot restart, or
	// for guilds large enough that disgo hasn't chunked every member
	// yet), OldMember is the zero value: empty role list, nil timeout,
	// nil nick, zero User.ID. Diffing against a zero value would produce
	// false "X changed from nothing to current value" entries for the
	// fields whose old state matters (nick, roles), so we bail those.
	// Timeout-add is the exception — a non-nil new timeout is unambiguous
	// (the member IS timed out right now) and that's the single highest-
	// severity member event admins audit, especially post-restart. We
	// still can't safely emit timeout-clear on cold cache because a nil
	// new state is indistinguishable from "never timed out".
	if old.User.ID == 0 {
		if updated.CommunicationDisabledUntil != nil {
			emit(audit.EventMemberTimeoutAdd, map[string]any{"timeout_until": updated.CommunicationDisabledUntil})
		}
		return
	}

	if !nickEqual(old.Nick, updated.Nick) {
		emit(audit.EventMemberNickChange, map[string]any{
			"nick_before": utils.RefDefault(old.Nick, ""),
			"nick_after":  utils.RefDefault(updated.Nick, ""),
		})
	}

	if !slices.Equal(old.RoleIDs, updated.RoleIDs) {
		added := diffRoles(updated.RoleIDs, old.RoleIDs)
		removed := diffRoles(old.RoleIDs, updated.RoleIDs)
		if len(added) > 0 || len(removed) > 0 {
			emit(audit.EventMemberRoleChange, map[string]any{
				"roles_added":   resolveRoles(e.Client(), e.GuildID, added),
				"roles_removed": resolveRoles(e.Client(), e.GuildID, removed),
			})
		}
	}

	oldTimeout := old.CommunicationDisabledUntil
	newTimeout := updated.CommunicationDisabledUntil
	switch {
	case oldTimeout == nil && newTimeout != nil:
		emit(audit.EventMemberTimeoutAdd, map[string]any{"timeout_until": newTimeout})
	case oldTimeout != nil && newTimeout == nil:
		emit(audit.EventMemberTimeoutClear, map[string]any{})
	case oldTimeout != nil && newTimeout != nil && !oldTimeout.Equal(*newTimeout):
		// Timeout duration extended / shortened — count as a fresh add
		// since the prior one was effectively replaced.
		emit(audit.EventMemberTimeoutAdd, map[string]any{"timeout_until": newTimeout})
	}
	// GuildMemberUpdate also fires for presence/avatar/etc. — those leave
	// all four branches above silent, which is the correct outcome.
}

func resolveRoles(client *bot.Client, guildID snowflake.ID, ids []snowflake.ID) []map[string]any {
	out := make([]map[string]any, len(ids))
	for i, id := range ids {
		out[i] = map[string]any{
			"id":   id.String(),
			"name": audit.ResolveRoleName(client, guildID, id),
		}
	}
	return out
}

func nickEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// diffRoles returns IDs in a but not in b.
func diffRoles(a, b []snowflake.ID) []snowflake.ID {
	bSet := make(map[snowflake.ID]struct{}, len(b))
	for _, id := range b {
		bSet[id] = struct{}{}
	}
	var out []snowflake.ID
	for _, id := range a {
		if _, ok := bSet[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

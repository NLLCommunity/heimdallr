package listeners

import (
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/audit"
	banIx "github.com/NLLCommunity/heimdallr/interactions/ban"
	"github.com/NLLCommunity/heimdallr/utils"
)

// OnAuditMemberBan records a guild.ban audit log entry. Runs alongside the
// existing OnMemberBan listener (which handles moderator-channel notification)
// — disgo allows multiple listeners on the same event.
//
// Routed through LogPending because the gateway event provides the
// banned-user reason via REST GetBan but not the moderator who pressed
// the button. Native audit log enrichment fills in only the actor; the
// reason from REST is more complete than what native audit truncates to,
// and is gated as non-enrichable in the pending whitelist.
func OnAuditMemberBan(e *events.GuildBan) {
	target := e.User.ID

	auditDetails := map[string]any{
		"target_username": e.User.Username,
	}

	auditEntry := audit.Entry{
		GuildID:    e.GuildID,
		EventType:  audit.EventGuildBan,
		ActorKind:  audit.ActorUnknown,
		TargetID:   &target,
		TargetKind: audit.TargetUser,
		Source:     audit.SourceGateway,
		Details:    auditDetails,
	}

	// REST fetch is best-effort — audit log entries must record the ban
	// either way, even if the reason is unavailable. This is a separate
	// REST call from the one OnMemberBan makes; bans are rare enough that
	// the duplicate request isn't worth coupling the two listeners over.
	if ban, err := e.Client().Rest.GetBan(e.GuildID, e.User.ID); err == nil && ban != nil {
		reason := utils.RefDefault(ban.Reason, "")
		banData := banIx.BanHandlerDataFromString(reason)

		auditEntry.Reason = banData.Reason
		if auditEntry.Reason == "" {
			auditEntry.Reason = reason
		}
		if banData.Message != "" {
			auditDetails["message"] = banData.Message
		}
		if banData.Duration != "" {
			auditDetails["duration"] = banData.Duration
		}
		if banData.BanningUserID != 0 {
			bid := banData.BanningUserID
			auditEntry.ActorID = &bid
			auditEntry.ActorKind = audit.ActorUser
		}
	}

	audit.LogPending(auditEntry, []audit.EnrichField{audit.EnrichActor})
}

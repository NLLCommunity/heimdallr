package listeners

import (
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/audit"
)

// OnAuditGuildUnban records a guild.unban entry. Routed through LogPending
// because the gateway event provides only the unbanned user — the moderator
// who lifted the ban and any reason come from the native audit log.
func OnAuditGuildUnban(e *events.GuildUnban) {
	target := e.User.ID
	details := map[string]any{
		"target_username": e.User.Username,
	}
	audit.LogPending(audit.Entry{
		GuildID:    e.GuildID,
		EventType:  audit.EventGuildUnban,
		ActorKind:  audit.ActorUnknown,
		TargetID:   &target,
		TargetKind: audit.TargetUser,
		Source:     audit.SourceGateway,
		Details:    details,
	}, []audit.EnrichField{audit.EnrichActor, audit.EnrichReason})
}

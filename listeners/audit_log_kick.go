package listeners

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
)

// OnAuditLogKick is the legacy moderator-channel kick notification listener.
// It posts a "User X was kicked by Y" message to the configured moderator
// channel and does NOT write to the bot's audit log — that row is written
// by OnAuditNativeEnrichment (audit_native_enrichment.go). The gateway
// leave event for the kicked member is not audited, so no suppression of
// a duplicate row is required. Both listeners subscribe to the same
// GuildAuditLogEntryCreate event; disgo dispatches to both.
func OnAuditLogKick(e *events.GuildAuditLogEntryCreate) {
	entry := e.AuditLogEntry

	if entry.ActionType != discord.AuditLogEventMemberKick {
		return
	}
	if entry.TargetID == nil {
		return
	}

	targetUser, err := e.Client().Rest.GetUser(*entry.TargetID)
	if err != nil {
		return
	}
	user, err := e.Client().Rest.GetUser(entry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", entry.UserID)
		return
	}

	if pruned, _ := model.IsMemberPruned(e.GuildID, targetUser.ID); pruned {
		return
	}

	reason := ""
	if entry.Reason != nil {
		reason = fmt.Sprintf("\n>>> %s", *entry.Reason)
	}

	msg := fmt.Sprintf(
		"User %s (`%d`) was kicked by %s.%s", targetUser.Username, targetUser.ID,
		user.Mention(),
		reason,
	)

	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	_, err = e.Client().Rest.CreateMessage(
		guildSettings.ModeratorChannel, discord.NewMessageCreate().
			WithContent(msg).
			WithAllowedMentions(
				&discord.AllowedMentions{
					RepliedUser: false,
				},
			),
	)

	if err != nil {
		slog.Error("Failed to send audit log message.", "err", err, "guild", e.GuildID)
	}
}

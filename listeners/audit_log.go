package listeners

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/globals"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func OnAuditLog(e *events.GuildAuditLogEntryCreate) {
	entry := e.AuditLogEntry

	msg := ""
	switch entry.ActionType {
	case discord.AuditLogEventMemberKick:
		targetUser, err := e.Client().Rest().GetUser(*entry.TargetID)
		if err != nil {
			return
		}
		user, err := e.Client().Rest().GetUser(entry.UserID)
		if err != nil {
			slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", entry.UserID)
			return
		}

		if _, ok := globals.ExcludedFromModKickLog[targetUser.ID]; ok {
			// User is excluded from mod kick log, likely because they were pruned.
			// Remove from excluded list and don't log.
			delete(globals.ExcludedFromModKickLog, targetUser.ID)
			return
		}

		msg = fmt.Sprintf("User %s (`%d`) was kicked by %s.%s", targetUser.Username, targetUser.ID,
			user.Mention(),
			utils.Iif(entry.Reason != nil, fmt.Sprintf("\n\n>>> %s", *entry.Reason), ""))

	default:
		return
	}

	if msg == "" {
		return
	}

	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	_, err = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
		SetContent(msg).
		SetAllowedMentions(&discord.AllowedMentions{
			RepliedUser: false,
		}).
		Build())

	if err != nil {
		slog.Error("Failed to send audit log message.", "err", err, "guild", e.GuildID)
	}
}

package listeners

import (
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
	"log/slog"
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

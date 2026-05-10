package listeners

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
)

func OnAuditLogKick(e *events.GuildAuditLogEntryCreate) {
	entry := e.AuditLogEntry

	if entry.ActionType != discord.AuditLogEventMemberKick {
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

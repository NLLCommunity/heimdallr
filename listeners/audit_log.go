package listeners

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
)

func OnAuditLog(e *events.GuildAuditLogEntryCreate) {
	entry := e.AuditLogEntry

	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	msg := ""
	switch entry.ActionType {
	case discord.AuditLogEventMemberKick:
		msg = onKickEvent(e)

	case discord.AuditLogEventMemberBanAdd:
		msg = onBanAddEvent(e)

	case discord.AuditLogEventMemberBanRemove:
		msg = onBanRemoveEvent(e)

	case discord.AuditLogEventMemberRoleUpdate:
		msg = onMemberRoleUpdateEvent(e)

	case discord.AuditLogEventBotAdd:
		msg = onBotAddEvent(e)

	case discord.AuditLogEventChannelCreate:
		msg = onChannelCreateEvent(e)

	case discord.AuditLogEventChannelUpdate:
		msg = onChannelUpdateEvent(e)

	case discord.AuditLogEventChannelDelete:
		msg = onChannelDeleteEvent(e)

	case discord.AuditLogEventChannelOverwriteCreate:
		msg = onChannelOverwriteCreateEvent(e)

	case discord.AuditLogEventChannelOverwriteUpdate:
		msg = onChannelOverwriteUpdateEvent(e)

	case discord.AuditLogEventChannelOverwriteDelete:
		msg = onChannelOverwriteDeleteEvent(e)

	case discord.AuditLogEventMemberPrune:
		msg = onMemberPruneEvent(e)

	case discord.AuditLogEventMemberUpdate:
		msg = onMemberUpdateEvent(e)

	default:
		return
	}

	if msg == "" {
		return
	}

	_, err = e.Client().Rest().CreateMessage(guildSettings.AuditLogChannel, discord.NewMessageCreateBuilder().
		SetContent(msg).
		SetAllowedMentions(&discord.AllowedMentions{
			RepliedUser: false,
		}).
		Build())

	if err != nil {
		slog.Error("Failed to send audit log message.", "err", err, "guild", e.GuildID)
	}
}

func onKickEvent(e *events.GuildAuditLogEntryCreate) string {
	user, targetUser, err := getUserAndTargetUser(e)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	return fmt.Sprintf("%s kicked %s (`%d`).%s",
		user.Mention(),
		targetUser.Username,
		targetUser.ID,
		utils.Iif(e.AuditLogEntry.Reason != nil, fmt.Sprintf("\n\n>>> %s", *e.AuditLogEntry.Reason), ""),
	)
}

func onBanAddEvent(e *events.GuildAuditLogEntryCreate) string {
	user, targetUser, err := getUserAndTargetUser(e)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	return fmt.Sprintf("%s banned %s (`%d`).%s",
		user.Mention(),
		targetUser.Username,
		targetUser.ID,
		utils.Iif(e.AuditLogEntry.Reason != nil, fmt.Sprintf("\n\n>>> %s", *e.AuditLogEntry.Reason), ""),
	)
}

func onBanRemoveEvent(e *events.GuildAuditLogEntryCreate) string {
	user, targetUser, err := getUserAndTargetUser(e)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	return fmt.Sprintf("%s unbanned %s (`%d`).%s",
		user.Mention(),
		targetUser.Username,
		targetUser.ID,
		utils.Iif(e.AuditLogEntry.Reason != nil, fmt.Sprintf("\n\n>>> %s", *e.AuditLogEntry.Reason), ""),
	)
}

func onMemberRoleUpdateEvent(e *events.GuildAuditLogEntryCreate) string {
	user, targetUser, err := getUserAndTargetUser(e)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	var addedRoles, removedRoles []string
	for _, change := range e.AuditLogEntry.Changes {
		if change.Key == "$add" {
			var d []struct {
				Name string       `json:"name"`
				ID   snowflake.ID `json:"id"`
			}
			err := json.Unmarshal(change.NewValue, &d)
			if err != nil {
				fmt.Println("Failed to unmarshal role change: ", err)
				continue
			}

			for _, role := range d {
				addedRoles = append(addedRoles, role.Name)
			}
		} else if change.Key == "$remove" {
			var d []struct {
				Name string       `json:"name"`
				ID   snowflake.ID `json:"id"`
			}
			err := json.Unmarshal(change.NewValue, &d)
			if err != nil {
				fmt.Println("Failed to unmarshal role change: ", err)
				continue
			}

			for _, role := range d {
				removedRoles = append(removedRoles, role.Name)
			}
		}

	}

	addedRolesStr := strings.Join(addedRoles, ", ")
	removedRolesStr := strings.Join(removedRoles, ", ")

	return fmt.Sprintf("%s updated the roles of %s (%d)\n\n%s%s%s",
		user.Mention(),
		targetUser.Username,
		targetUser.ID,
		utils.Iif(addedRolesStr == "", "", fmt.Sprintf("**Added roles:** %s\n", addedRolesStr)),
		// ↓ Add a newline if both added and removed roles are present
		utils.Iif(addedRolesStr != "" && removedRolesStr != "", "\n", ""),
		utils.Iif(removedRolesStr == "", "", fmt.Sprintf("**Removed roles:** %s\n", removedRolesStr)),
	)
}

func onBotAddEvent(e *events.GuildAuditLogEntryCreate) string {
	user, targetUser, err := getUserAndTargetUser(e)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	return fmt.Sprintf("%s added bot %s (%d) to the server", user.Mention(), targetUser.Username, targetUser.ID)
}

func onChannelCreateEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	channel, err := e.Client().Rest().GetChannel(*e.AuditLogEntry.TargetID)
	if err != nil {
		slog.Warn("Failed to get channel for audit log entry.", "err", err, "channel_id", *e.AuditLogEntry.TargetID)
		return ""
	}

	return fmt.Sprintf("%s created channel *%s* <#%d>", user.Mention(), channel.Name(), channel.ID())
}

func onChannelUpdateEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	channel, err := e.Client().Rest().GetChannel(*e.AuditLogEntry.TargetID)
	if err != nil {
		slog.Warn("Failed to get channel for audit log entry.", "err", err, "channel_id", *e.AuditLogEntry.TargetID)
		return ""
	}

	return fmt.Sprintf("%s updated channel *%s* <#%d>", user.Mention(), channel.Name(), channel.ID())
}

func onChannelDeleteEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	channel, err := e.Client().Rest().GetChannel(*e.AuditLogEntry.TargetID)
	if err != nil {
		slog.Warn("Failed to get channel for audit log entry.", "err", err, "channel_id", *e.AuditLogEntry.TargetID)
		return ""
	}

	return fmt.Sprintf("%s deleted channel *%s* <#%d>", user.Mention(), channel.Name(), channel.ID())
}

func onChannelOverwriteCreateEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	channel, err := e.Client().Rest().GetChannel(*e.AuditLogEntry.TargetID)
	if err != nil {
		slog.Warn("Failed to get channel for audit log entry.", "err", err, "channel_id", *e.AuditLogEntry.TargetID)
		return ""
	}

	return fmt.Sprintf("%s created overwrite in channel *%s* <#%d>", user.Mention(), channel.Name(), channel.ID())
}

func onChannelOverwriteUpdateEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	channel, err := e.Client().Rest().GetChannel(*e.AuditLogEntry.TargetID)
	if err != nil {
		slog.Warn("Failed to get channel for audit log entry.", "err", err, "channel_id", *e.AuditLogEntry.TargetID)
		return ""
	}

	for _, change := range e.AuditLogEntry.Changes {
		fmt.Println("old: ", string(change.OldValue))
		fmt.Println("new: ", string(change.NewValue))

	}

	return fmt.Sprintf("%s updated overwrite for in channel *%s* <#%d>", user.Mention(), channel.Name(), channel.ID())
}

func onChannelOverwriteDeleteEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	channel, err := e.Client().Rest().GetChannel(*e.AuditLogEntry.TargetID)
	if err != nil {
		slog.Warn("Failed to get channel for audit log entry.", "err", err, "channel_id", *e.AuditLogEntry.TargetID)
		return ""
	}

	return fmt.Sprintf("%s deleted overwrite for in channel *%s* <#%d>", user.Mention(), channel.Name(), channel.ID())
}

func onMemberPruneEvent(e *events.GuildAuditLogEntryCreate) string {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	return fmt.Sprintf("%s pruned %s members.", user.Mention(), *e.AuditLogEntry.Options.MembersRemoved)
}

func onMemberUpdateEvent(e *events.GuildAuditLogEntryCreate) string {
	user, targetUser, err := getUserAndTargetUser(e)
	if err != nil {
		slog.Warn("Failed to get user for audit log entry.", "err", err, "user_id", e.AuditLogEntry.UserID)
		return ""
	}

	msg := fmt.Sprintf("%s updated the profile of %s (`%d`).\n\n>>> ",
		user.Mention(),
		targetUser.Username,
		targetUser.ID,
	)

	for _, change := range e.AuditLogEntry.Changes {
		fmt.Println(change)
		if change.Key == "nick" {
			var old, new string
			err := json.Unmarshal(change.OldValue, &old)
			if err != nil {
				old = " "
			}
			err = json.Unmarshal(change.NewValue, &new)
			if err != nil {
				new = " "
			}
			msg += fmt.Sprintf("Nickname changed from `%s` to `%s`.\n", old, new)
		}
	}

	return msg
}

func getUserAndTargetUser(e *events.GuildAuditLogEntryCreate) (*discord.User, *discord.User, error) {
	user, err := e.Client().Rest().GetUser(e.AuditLogEntry.UserID)
	if err != nil {
		return nil, nil, err
	}
	targetUser, err := e.Client().Rest().GetUser(*e.AuditLogEntry.TargetID)
	if err != nil {
		return nil, nil, err
	}
	return user, targetUser, nil
}

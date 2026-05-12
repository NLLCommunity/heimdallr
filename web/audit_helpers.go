package web

import (
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/model"
)

// logSettingsUpdate records a settings.update audit log entry. Each
// settings save handler calls this on its happy path with the section name
// (the form/group that was saved, e.g. "mod_channel") and an optional map of
// fields that were changed.
//
// session may be nil — checkGuildAdmin would normally have rejected the
// request before reaching the save path, but log a system actor as a
// defensive fallback rather than panicking.
func logSettingsUpdate(
	session *model.DashboardSession,
	guildID snowflake.ID,
	section string,
	fields map[string]any,
) {
	details := map[string]any{"section": section}
	for k, v := range fields {
		details[k] = v
	}

	gid := guildID
	entry := audit.Entry{
		GuildID:    guildID,
		EventType:  audit.EventSettingsUpdate,
		ActorKind:  audit.ActorSystem,
		TargetID:   &gid,
		TargetKind: audit.TargetGuild,
		Source:     audit.SourceWeb,
		Details:    details,
	}
	if session != nil {
		uid := session.UserID
		entry.ActorID = &uid
		entry.ActorKind = audit.ActorUser
		// Record the dashboard user's username at write time so the
		// viewer renders @username instead of the raw snowflake when
		// the disgo member cache is cold (common after a bot restart).
		details["actor_username"] = session.Username
	}
	audit.Log(entry)
}

// logPostUpdate records a web.post.{create,update,delete} audit log entry.
func logPostUpdate(
	session *model.DashboardSession,
	guildID snowflake.ID,
	eventType audit.EventType,
	postID uint,
	postName string,
) {
	details := map[string]any{
		"post_id": postID,
	}
	if postName != "" {
		details["post_name"] = postName
	}

	entry := audit.Entry{
		GuildID:    guildID,
		EventType:  eventType,
		ActorKind:  audit.ActorSystem,
		TargetKind: audit.TargetNone,
		Source:     audit.SourceWeb,
		Details:    details,
	}
	if session != nil {
		uid := session.UserID
		entry.ActorID = &uid
		entry.ActorKind = audit.ActorUser
		details["actor_username"] = session.Username
	}
	audit.Log(entry)
}

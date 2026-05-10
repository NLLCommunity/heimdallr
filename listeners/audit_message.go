package listeners

import (
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/audit"
)

// OnAuditMessageUpdate records a message edit. The OldMessage on the event
// reflects whatever disgo had cached — when there's a cache miss we record
// before_content as empty rather than failing.
//
// Self-edits don't have a meaningful "actor different from the author"
// case, so this is committed via Log (no enrichment expected). Discord
// also doesn't fire a native audit log entry for self-edits.
func OnAuditMessageUpdate(e *events.GuildMessageUpdate) {
	msg := e.Message
	authorID := msg.Author.ID
	messageID := e.MessageID

	beforeContent := e.OldMessage.Content
	afterContent := msg.Content
	if beforeContent == afterContent {
		// Discord fires MessageUpdate for embed resolution, link unfurl,
		// pin, etc. without a content change. Skip those.
		return
	}

	details := map[string]any{
		"channel_id":     e.ChannelID.String(),
		"author_id":      authorID.String(),
		"before_content": beforeContent,
		"after_content":  afterContent,
		// actor_username here covers the case where the author has left
		// the guild and the member cache misses. The actor IS the author
		// for an edit (Discord does not let other users edit messages).
		"actor_username": msg.Author.Username,
	}

	audit.Log(audit.Entry{
		GuildID:    e.GuildID,
		EventType:  audit.EventMessageEdit,
		ActorID:    &authorID,
		ActorKind:  audit.ActorUser,
		TargetID:   &messageID,
		TargetKind: audit.TargetMessage,
		Source:     audit.SourceGateway,
		Details:    details,
	})
}

// OnAuditMessageDelete records a message deletion. Routed through
// LogPending because Discord's native audit log identifies the moderator
// when a message is deleted by someone other than the author. If the
// deletion is self-initiated the pending entry just commits unenriched
// after the TTL with the author as actor.
func OnAuditMessageDelete(e *events.GuildMessageDelete) {
	messageID := e.MessageID

	details := map[string]any{
		"channel_id": e.ChannelID.String(),
	}

	// e.Message is the cached pre-delete copy when available.
	var authorID *snowflake.ID
	if e.Message.Author.ID != 0 {
		id := e.Message.Author.ID
		details["author_id"] = id.String()
		details["author_username"] = e.Message.Author.Username
		details["before_content"] = e.Message.Content
		// For self-deletes, actor == author. Store actor_username so the
		// viewer renders a name on cache miss. For moderator-initiated
		// deletions, native audit log enrichment overwrites this field
		// (and ActorID/ActorKind) with the moderator's data.
		details["actor_username"] = e.Message.Author.Username
		authorID = &id
	}

	entry := audit.Entry{
		GuildID:    e.GuildID,
		EventType:  audit.EventMessageDelete,
		TargetID:   &messageID,
		TargetKind: audit.TargetMessage,
		Source:     audit.SourceGateway,
		Details:    details,
	}
	if authorID != nil {
		entry.ActorID = authorID
		entry.ActorKind = audit.ActorUser
	} else {
		entry.ActorKind = audit.ActorUnknown
	}

	audit.LogPending(entry, []audit.EnrichField{audit.EnrichActor, audit.EnrichReason})
}

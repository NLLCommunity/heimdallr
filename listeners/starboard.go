package listeners

import (
	"github.com/NLLCommunity/heimdallr/starboard"
	"github.com/disgoorg/disgo/events"
)

func OnStarboardReactionAdd(e *events.GuildMessageReactionAdd) {
	if starboard.Default != nil {
		starboard.Default.Notify(e.GuildID, e.ChannelID, e.MessageID, false)
	}
}
func OnStarboardReactionRemove(e *events.GuildMessageReactionRemove) {
	if starboard.Default != nil {
		starboard.Default.Notify(e.GuildID, e.ChannelID, e.MessageID, false)
	}
}
func OnStarboardReactionRemoveEmoji(e *events.GuildMessageReactionRemoveEmoji) {
	if starboard.Default != nil {
		starboard.Default.Notify(e.GuildID, e.ChannelID, e.MessageID, false)
	}
}
func OnStarboardReactionRemoveAll(e *events.GuildMessageReactionRemoveAll) {
	if starboard.Default != nil {
		starboard.Default.Notify(e.GuildID, e.ChannelID, e.MessageID, false)
	}
}
func OnStarboardMessageUpdate(e *events.GuildMessageUpdate) {
	if starboard.Default != nil {
		starboard.Default.NotifyUpdate(e.GuildID, e.ChannelID, e.MessageID)
	}
}

// disgo dispatches GuildMessageDelete for each ID in MESSAGE_DELETE_BULK,
// so this handler covers both individual and bulk source/copy deletions.
func OnStarboardMessageDelete(e *events.GuildMessageDelete) {
	if starboard.Default != nil {
		starboard.Default.Notify(e.GuildID, e.ChannelID, e.MessageID, true)
	}
}

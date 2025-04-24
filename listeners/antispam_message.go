package listeners

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agnivade/levenshtein"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/jellydator/ttlcache/v3"

	"github.com/NLLCommunity/heimdallr/model"
)

const minMessageLength = 10
const maxLevenshteinDistance = 5
const maxMessages = 20

var whitespaceReplacer = strings.NewReplacer(
	" ", "",
	" ", "",
	" ", "",
	"\t", "",
	"\u200b", "",
	" ", "",
	" ", "",
	" ", "",
)

var userMessages = ttlcache.New[string, userMessagesInfo](
	ttlcache.WithTTL[string, userMessagesInfo](60 * time.Second))

type userMessagesInfo struct {
	Score    int
	Messages []*messageDetails
}

type messageDetails struct {
	Content   string
	ChannelID snowflake.ID
	MessageID snowflake.ID
}

func OnAntispamMessageCreate(e *events.GuildMessageCreate) {
	uHash := fmt.Sprintf("%d:%d", e.GuildID, e.Message.Author.ID)
	guildSettings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		slog.Warn("Failed to get guild settings.", "err", err, "guild_id", e.GuildID)
		return
	}

	cooldown := time.Duration(guildSettings.AntiSpamCooldownSeconds) * time.Second

	if !guildSettings.AntiSpamEnabled {
		return
	}

	if !userMessages.Has(uHash) {
		createMessageInfoForUser(uHash, e.Message, cooldown)
		return
	}

	messagesInfo := userMessages.Get(uHash)
	if messagesInfo == nil {
		slog.Warn("Failed to get messages info for user.", "guild", e.GuildID, "user", e.Message.Author.ID)
		return
	}

	info := messagesInfo.Value()
	matchesPreviousMessage := compareToPreviousMessages(e.Message, info)

	if len(info.Messages) >= maxMessages {
		info.Messages = info.Messages[1:]
	}
	info.Messages = append(info.Messages, createMessageDetails(e.Message))

	if matchesPreviousMessage {
		info.Score++
	}

	userMessages.Set(uHash, info, cooldown)

	if info.Score >= guildSettings.AntiSpamCount {
		timeoutUser(e, guildSettings, info)
	}

}

func timeoutUser(e *events.GuildMessageCreate, guildSettings *model.GuildSettings, info userMessagesInfo) {
	userID := e.Message.Author.ID
	cooldown := time.Duration(guildSettings.AntiSpamCooldownSeconds) * time.Second
	cutoffTime := time.Now().Add(-cooldown)

	expiry := time.Now().Add(24 * time.Hour)
	_, err := e.Client().Rest().UpdateMember(e.GuildID, userID, discord.MemberUpdate{
		CommunicationDisabledUntil: json.NewNullablePtr(expiry),
	}, rest.WithReason("User timed out due to anti-spam settings."))

	if err != nil {
		slog.Error("Failed to timeout user.", "err", err, "guild", e.GuildID, "user", userID)
		return
	}

	var removableMessageIDs []*messageDetails
	for _, m := range info.Messages {
		if m.MessageID.Time().Before(cutoffTime) {
			continue
		}

		removableMessageIDs = append(removableMessageIDs, m)
	}

	for _, m := range removableMessageIDs {
		err := e.Client().Rest().DeleteMessage(m.ChannelID, m.MessageID)
		if err != nil {
			slog.Error("Failed to delete message.", "err", err, "guild", e.GuildID, "channel", m.ChannelID, "message", m.MessageID)
		}
	}

	if guildSettings.ModeratorChannel == 0 {
		return
	}

	adminMessage := fmt.Sprintf(
		"User %s has been timed out for spamming. Deleted %d messages.\n\nTriggering message:\n>>> %s",
		e.Message.Author.Mention(), len(removableMessageIDs), e.Message.Content)

	_, err = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
		SetContent(adminMessage).
		Build())

	if err != nil {
		slog.Error("Failed to send timeout message to moderator channel.",
			"err", err,
			"guild", e.GuildID,
			"channel", guildSettings.ModeratorChannel,
			"user", e.Message.Author.ID)
	}
}

func compareToPreviousMessages(m discord.Message, info userMessagesInfo) bool {
	if len(info.Messages) == 0 {
		return false
	}
	for _, mInfo := range info.Messages {
		// Remove all whitespace from the message content
		currentMessage := whitespaceReplacer.Replace(m.Content)
		if len(currentMessage) < minMessageLength {
			return false
		}

		prevMessage := whitespaceReplacer.Replace(mInfo.Content)

		distance := levenshtein.ComputeDistance(currentMessage, prevMessage)
		if distance < maxLevenshteinDistance && m.ChannelID != mInfo.ChannelID {
			// Return true if these are similar messages across channels
			return true
		}
	}
	return false
}

func createMessageInfoForUser(uHash string, m discord.Message, ttl time.Duration) {
	userMessages.Set(uHash,
		userMessagesInfo{
			Messages: []*messageDetails{createMessageDetails(m)},
		},
		ttl)
}

func createMessageDetails(m discord.Message) *messageDetails {
	return &messageDetails{
		Content:   m.Content,
		ChannelID: m.ChannelID,
		MessageID: m.ID,
	}
}

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
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"
	"github.com/jellydator/ttlcache/v3"

	"github.com/NLLCommunity/heimdallr/model"
)

const minMessageLength = 10
const maxLevenshteinDistancePercent = 5
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
	ttlcache.WithTTL[string, userMessagesInfo](60 * time.Second),
)

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

	if len(info.Messages) >= maxMessages {
		info.Messages = info.Messages[1:]
	}

	messageDetails := createMessageDetails(e.Message)
	info.Messages = append(info.Messages, messageDetails)

	// Check if this message is similar to previous messages across channels
	// matching minimum length and Levenshtein distance thresholds.
	matchesPreviousMessage := compareToPreviousMessages(messageDetails, info)
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

	expiry := time.Now().Add(time.Duration(guildSettings.AntiSpamTimeoutMinutes) * time.Minute)
	_, err := e.Client().Rest.UpdateMember(
		e.GuildID, userID, discord.MemberUpdate{
			CommunicationDisabledUntil: omit.NewPtr(expiry),
		}, rest.WithReason("User timed out due to anti-spam settings."),
	)

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
		err := e.Client().Rest.DeleteMessage(m.ChannelID, m.MessageID)
		if err != nil {
			slog.Error(
				"Failed to delete message.", "err", err, "guild", e.GuildID, "channel", m.ChannelID, "message",
				m.MessageID,
			)
		}
	}

	if guildSettings.ModeratorChannel == 0 {
		return
	}

	timeoutMessage := createTimeoutMessage(e, removableMessageIDs, len(removableMessageIDs))

	_, err = e.Client().Rest.CreateMessage(
		guildSettings.ModeratorChannel, timeoutMessage,
	)

	if err != nil {
		slog.Error(
			"Failed to send timeout message to moderator channel.",
			"err", err,
			"guild", e.GuildID,
			"channel", guildSettings.ModeratorChannel,
			"user", e.Message.Author.ID,
		)
	}
}

// Discord V2 component message limits.
const (
	maxTopLevelComponents = 10
	maxTotalComponents    = 40
	maxTotalTextLength    = 4000
	maxPerMessageContent  = 500
	truncationMarker      = "…"
)

func createTimeoutMessage(e *events.GuildMessageCreate, msgs []*messageDetails, deletedCount int) discord.MessageCreate {
	summary := fmt.Sprintf("User %s has been timed out for spamming. Deleted %d messages.", e.Message.Author.Username, deletedCount)

	components := []discord.LayoutComponent{discord.NewTextDisplay(summary)}
	totalComponents := 1
	totalText := len(summary)

	const omissionReserveText = 64
	omitted := 0

	for i, m := range msgs {
		content := m.Content
		if len(content) > maxPerMessageContent {
			content = content[:maxPerMessageContent] + truncationMarker
		}
		contentText := fmt.Sprintf(">>> %s", content)
		channelText := fmt.Sprintf("-# Channel: <#%s>", m.ChannelID)

		// Each entry adds 1 container + 2 text displays.
		const addComponents = 3
		addText := len(contentText) + len(channelText)

		// Reserve budget for a possible "N omitted" notice if there are more entries after this one.
		reserveComponents := 0
		reserveText := 0
		if i < len(msgs)-1 {
			reserveComponents = 1
			reserveText = omissionReserveText
		}

		if len(components)+1+reserveComponents > maxTopLevelComponents ||
			totalComponents+addComponents+reserveComponents > maxTotalComponents ||
			totalText+addText+reserveText > maxTotalTextLength {
			omitted = len(msgs) - i
			break
		}

		components = append(components, discord.NewContainer(
			discord.NewTextDisplay(contentText),
			discord.NewTextDisplay(channelText),
		))
		totalComponents += addComponents
		totalText += addText
	}

	if omitted > 0 {
		components = append(components, discord.NewTextDisplayf("-# … and %d more message(s) omitted.", omitted))
	}

	return discord.NewMessageCreateV2(components...)
}

func compareToPreviousMessages(details *messageDetails, info userMessagesInfo) bool {
	if len(info.Messages) == 0 {
		return false
	}

	slog.Debug("Comparing message to previous messages.", "current_message", details.Content, "previous_messages_count", len(info.Messages))

	for _, mInfo := range info.Messages {
		// Remove all whitespace from the message content
		currentMessage := whitespaceReplacer.Replace(details.Content)
		if len(currentMessage) < minMessageLength {
			return false
		}

		prevMessage := whitespaceReplacer.Replace(mInfo.Content)

		distance := levenshtein.ComputeDistance(currentMessage, prevMessage)
		maxLevenshteinDistance := int(float64(len(currentMessage)) * maxLevenshteinDistancePercent / 100)
		if distance <= maxLevenshteinDistance && details.ChannelID != mInfo.ChannelID {
			// Return true if these are similar messages across channels
			slog.Info("Found similar message across channels.", "current_message", details.Content, "previous_message", mInfo.Content, "distance", distance)
			return true
		} else {
			slog.Debug("Messages are not similar enough or are in the same channel.", "current_message", details.Content, "previous_message", mInfo.Content, "distance", distance)
		}
	}
	return false
}

func createMessageInfoForUser(uHash string, m discord.Message, ttl time.Duration) {
	userMessages.Set(
		uHash,
		userMessagesInfo{
			Messages: []*messageDetails{createMessageDetails(m)},
		},
		ttl,
	)
}

func createMessageDetails(m discord.Message) *messageDetails {
	return &messageDetails{
		Content:   messageWithAttachmentInfo(m),
		ChannelID: m.ChannelID,
		MessageID: m.ID,
	}
}

func messageWithAttachmentInfo(m discord.Message) string {
	atts := make([]string, len(m.Attachments))
	for i, att := range m.Attachments {
		width := 0
		height := 0
		if att.Width != nil {
			width = *att.Width
		}
		if att.Height != nil {
			height = *att.Height
		}

		atts[i] = fmt.Sprintf("%s %dx%d %d", att.Filename, width, height, att.Size)
	}

	if len(atts) == 0 {
		return m.Content
	}

	return fmt.Sprintf("%s\nAttachments:\n%s", m.Content, strings.Join(atts, "\n"))
}

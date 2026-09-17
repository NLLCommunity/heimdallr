package starboard

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/NLLCommunity/heimdallr/interactions/quote"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const reactionPageSize = 100

const (
	recoveryPageSize    = 100
	maxRecoveryPages    = 10
	recoverySafetyDelay = 10 * time.Minute
)

type discordREST interface {
	GetChannel(channelID snowflake.ID, opts ...rest.RequestOpt) (discord.Channel, error)
	GetGuild(guildID snowflake.ID, withCounts bool, opts ...rest.RequestOpt) (*discord.RestGuild, error)
	GetMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) (*discord.Message, error)
	GetMessages(channelID snowflake.ID, around snowflake.ID, before snowflake.ID, after snowflake.ID, limit int, opts ...rest.RequestOpt) ([]discord.Message, error)
	CreateMessage(channelID snowflake.ID, messageCreate discord.MessageCreate, opts ...rest.RequestOpt) (*discord.Message, error)
	UpdateMessage(channelID snowflake.ID, messageID snowflake.ID, messageUpdate discord.MessageUpdate, opts ...rest.RequestOpt) (*discord.Message, error)
	DeleteMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	GetMember(guildID snowflake.ID, userID snowflake.ID, opts ...rest.RequestOpt) (*discord.Member, error)
	GetEmoji(guildID snowflake.ID, emojiID snowflake.ID, opts ...rest.RequestOpt) (*discord.Emoji, error)
	GetReactions(channelID snowflake.ID, messageID snowflake.ID, emoji string, reactionType discord.MessageReactionType, after int, limit int, opts ...rest.RequestOpt) ([]discord.User, error)
	AddReaction(channelID snowflake.ID, messageID snowflake.ID, emoji string, opts ...rest.RequestOpt) error
}

// NewDiscordTransport adapts disgo's REST client to the starboard service.
func NewDiscordTransport(client *bot.Client) Transport {
	return newDiscordTransportWithREST(client, client.Rest)
}

type discordTransport struct {
	client     *bot.Client
	rest       discordREST
	selfID     func() snowflake.ID
	quoteEmbed func(*discord.Message) discord.Embed
}

func newDiscordTransportWithREST(client *bot.Client, restClient discordREST) *discordTransport {
	transport := &discordTransport{client: client, rest: restClient}
	transport.selfID = func() snowflake.ID {
		if client == nil {
			return 0
		}
		if id := client.ID(); id != 0 {
			return id
		}
		return client.ApplicationID
	}
	transport.quoteEmbed = func(message *discord.Message) discord.Embed {
		return quote.CreateMessageQuoteEmbed(client, message, true)
	}
	return transport
}

func (t *discordTransport) ValidateBoard(board model.Starboard) error {
	channel, err := t.rest.GetChannel(board.ChannelID)
	if err != nil {
		return fmt.Errorf("fetch starboard destination: %w", err)
	}
	textChannel, ok := channel.(discord.GuildTextChannel)
	if !ok {
		return fmt.Errorf("starboard destination %d is not a guild text channel", board.ChannelID)
	}
	if textChannel.GuildID() != board.GuildID {
		return fmt.Errorf("starboard destination %d does not belong to guild %d", board.ChannelID, board.GuildID)
	}

	guild, err := t.rest.GetGuild(board.GuildID, false)
	if err != nil {
		return fmt.Errorf("fetch destination guild: %w", err)
	}
	botID := t.selfID()
	if botID == 0 {
		return fmt.Errorf("cannot determine bot user ID")
	}
	member, err := t.rest.GetMember(board.GuildID, botID)
	if err != nil {
		return fmt.Errorf("fetch bot member: %w", err)
	}
	required := discord.PermissionViewChannel |
		discord.PermissionSendMessages |
		discord.PermissionEmbedLinks |
		discord.PermissionReadMessageHistory |
		discord.PermissionAddReactions
	if memberPermissions(guild, member, textChannel).Missing(required) {
		return fmt.Errorf("bot is missing required starboard destination permissions")
	}

	if _, err := t.resolveEmoji(board.GuildID, board.Emoji, member); err != nil {
		return err
	}
	return nil
}

type resolvedEmoji struct {
	reaction string
	display  string
}

func (t *discordTransport) resolveEmoji(guildID snowflake.ID, configured string, member *discord.Member) (resolvedEmoji, error) {
	separator := strings.LastIndexByte(configured, ':')
	if separator < 0 {
		return resolvedEmoji{reaction: configured, display: configured}, nil
	}
	emojiID, err := snowflake.Parse(configured[separator+1:])
	if err != nil || emojiID == 0 {
		return resolvedEmoji{}, fmt.Errorf("invalid custom emoji %q", configured)
	}
	emoji, err := t.rest.GetEmoji(guildID, emojiID)
	if err != nil {
		return resolvedEmoji{}, fmt.Errorf("fetch custom emoji %d: %w", emojiID, err)
	}
	if emoji == nil || emoji.ID != emojiID || emoji.GuildID != guildID || emoji.Name == "" || !emoji.Available {
		return resolvedEmoji{}, fmt.Errorf("custom emoji %d is not usable in this guild", emojiID)
	}
	if len(emoji.Roles) > 0 {
		if member == nil {
			return resolvedEmoji{}, fmt.Errorf("custom emoji %d requires a guild role", emojiID)
		}
		usable := false
		for _, roleID := range emoji.Roles {
			if slices.Contains(member.RoleIDs, roleID) {
				usable = true
				break
			}
		}
		if !usable {
			return resolvedEmoji{}, fmt.Errorf("custom emoji %d is not usable by the bot", emojiID)
		}
	}
	return resolvedEmoji{reaction: emoji.Reaction(), display: emoji.Mention()}, nil
}

func memberPermissions(guild *discord.RestGuild, member *discord.Member, channel discord.GuildChannel) discord.Permissions {
	if guild == nil || member == nil {
		return 0
	}
	if guild.OwnerID == member.User.ID {
		return discord.PermissionsAll
	}
	var permissions discord.Permissions
	for _, role := range guild.Roles {
		if role.ID == guild.ID || slices.Contains(member.RoleIDs, role.ID) {
			permissions |= role.Permissions
		}
	}
	if permissions.Has(discord.PermissionAdministrator) {
		return discord.PermissionsAll
	}
	if overwrite, ok := channel.PermissionOverwrites().Role(guild.ID); ok {
		permissions &= ^overwrite.Deny
		permissions |= overwrite.Allow
	}
	var roleAllow, roleDeny discord.Permissions
	for _, roleID := range member.RoleIDs {
		if overwrite, ok := channel.PermissionOverwrites().Role(roleID); ok {
			roleAllow |= overwrite.Allow
			roleDeny |= overwrite.Deny
		}
	}
	permissions &= ^roleDeny
	permissions |= roleAllow
	if overwrite, ok := channel.PermissionOverwrites().Member(member.User.ID); ok {
		permissions &= ^overwrite.Deny
		permissions |= overwrite.Allow
	}
	return permissions
}

func (t *discordTransport) Source(guild, channel, message snowflake.ID) (*discord.Message, error) {
	sourceChannel, err := t.rest.GetChannel(channel)
	if err != nil {
		return nil, fmt.Errorf("fetch source channel: %w", err)
	}
	guildChannel, ok := sourceChannel.(discord.GuildChannel)
	if !ok || guildChannel.GuildID() != guild {
		return nil, fmt.Errorf("source channel %d does not belong to guild %d", channel, guild)
	}

	source, err := t.rest.GetMessage(channel, message)
	if err != nil {
		if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch source message: %w", err)
	}
	return source, nil
}

func (t *discordTransport) Eligible(board model.Starboard, message *discord.Message) (bool, snowflake.ID, error) {
	if message == nil {
		return false, 0, fmt.Errorf("source message is nil")
	}

	channel, err := t.rest.GetChannel(message.ChannelID)
	if err != nil {
		return false, 0, fmt.Errorf("fetch source channel: %w", err)
	}
	guildChannel, ok := channel.(discord.GuildChannel)
	if !ok || guildChannel.GuildID() != board.GuildID {
		return false, 0, fmt.Errorf("source channel %d does not belong to guild %d", message.ChannelID, board.GuildID)
	}

	parentID := message.ChannelID
	visibilityChannel := guildChannel
	if thread, isThread := channel.(discord.GuildThread); isThread {
		parent := thread.ParentID()
		if parent == nil {
			return false, 0, fmt.Errorf("source thread %d has no parent", message.ChannelID)
		}
		parentID = *parent
		if thread.Type() == discord.ChannelTypeGuildPrivateThread {
			return false, parentID, nil
		}
		parentChannel, err := t.rest.GetChannel(parentID)
		if err != nil {
			return false, parentID, fmt.Errorf("fetch source parent channel: %w", err)
		}
		var parentOK bool
		visibilityChannel, parentOK = parentChannel.(discord.GuildChannel)
		if !parentOK || visibilityChannel.GuildID() != board.GuildID {
			return false, parentID, fmt.Errorf("source parent channel %d does not belong to guild %d", parentID, board.GuildID)
		}
	}

	if message.Author.Bot || message.WebhookID != nil || message.ChannelID == board.ChannelID {
		return false, parentID, nil
	}

	guild, err := t.rest.GetGuild(board.GuildID, false)
	if err != nil {
		return false, parentID, fmt.Errorf("fetch source guild: %w", err)
	}
	if !everyoneCanRead(guild, visibilityChannel) {
		return false, parentID, nil
	}

	destination, err := t.rest.GetChannel(board.ChannelID)
	if err != nil {
		return false, parentID, fmt.Errorf("fetch starboard destination: %w", err)
	}
	destinationChannel, ok := destination.(discord.GuildMessageChannel)
	if !ok || destinationChannel.GuildID() != board.GuildID {
		return false, parentID, fmt.Errorf("starboard destination %d does not belong to guild %d", board.ChannelID, board.GuildID)
	}
	sourceNSFW, ok := guildChannelNSFW(visibilityChannel)
	if !ok {
		return false, parentID, nil
	}
	if sourceNSFW && !destinationChannel.NSFW() {
		return false, parentID, nil
	}
	return true, parentID, nil
}

func guildChannelNSFW(channel discord.GuildChannel) (bool, bool) {
	switch channel := channel.(type) {
	case discord.GuildMessageChannel:
		return channel.NSFW(), true
	case discord.GuildForumChannel:
		return channel.NSFW, true
	case discord.GuildMediaChannel:
		return channel.NSFW, true
	default:
		return false, false
	}
}

func everyoneCanRead(guild *discord.RestGuild, channel discord.GuildChannel) bool {
	if guild == nil {
		return false
	}
	var permissions discord.Permissions
	for _, role := range guild.Roles {
		if role.ID == guild.ID {
			permissions = role.Permissions
			break
		}
	}
	if permissions.Has(discord.PermissionAdministrator) {
		return true
	}
	if overwrite, ok := channel.PermissionOverwrites().Role(guild.ID); ok {
		permissions &= ^overwrite.Deny
		permissions |= overwrite.Allow
	}
	return permissions.Has(discord.PermissionViewChannel, discord.PermissionReadMessageHistory)
}

func (t *discordTransport) Publish(board model.Starboard, source *discord.Message, votes Votes, nonce string) (snowflake.ID, error) {
	emoji, err := t.boardEmoji(board)
	if err != nil {
		return 0, errors.Join(ErrNotSent, err)
	}
	embed, components := t.starboardPresentation(board, source, votes, emoji.display)
	embed.URL = recoveryMarker(discord.MessageURL(board.GuildID, source.ChannelID, source.ID), nonce)
	create := discord.NewMessageCreate().
		WithNonce(nonce).
		WithEnforceNonce(true).
		WithEmbeds(embed).
		WithComponents(components...).
		WithAllowedMentions(&discord.AllowedMentions{})
	created, err := t.rest.CreateMessage(board.ChannelID, create)
	if err != nil {
		if definitelyNotSent(err) {
			return 0, errors.Join(ErrNotSent, err)
		}
		return 0, fmt.Errorf("publish starboard message: %w", err)
	}
	if created == nil || created.ID == 0 {
		return 0, fmt.Errorf("publish starboard message returned no message")
	}
	if err := t.seedReactions(board.ChannelID, created.ID, emoji.reaction); err != nil {
		return created.ID, err
	}
	return created.ID, nil
}

func (t *discordTransport) Update(board model.Starboard, source *discord.Message, copyID snowflake.ID, votes Votes) error {
	emoji, err := t.boardEmoji(board)
	if err != nil {
		return err
	}
	embed, components := t.starboardPresentation(board, source, votes, emoji.display)
	update := discord.NewMessageUpdate().
		WithEmbeds(embed).
		WithComponents(components...).
		WithAllowedMentions(&discord.AllowedMentions{})
	if _, err := t.rest.UpdateMessage(board.ChannelID, copyID, update); err != nil {
		if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage) {
			return ErrNotFound
		}
		return fmt.Errorf("update starboard message: %w", err)
	}
	return t.seedReactions(board.ChannelID, copyID, emoji.reaction)
}

func (t *discordTransport) Delete(channel, message snowflake.ID) error {
	err := t.rest.DeleteMessage(channel, message)
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete starboard message: %w", err)
	}
	return nil
}

func (t *discordTransport) Recover(board model.Starboard, nonce string, since time.Time) (snowflake.ID, error) {
	startID := snowflake.New(since)
	botID := t.selfID()
	if botID == 0 {
		return 0, fmt.Errorf("cannot determine bot user ID")
	}
	var before snowflake.ID
	complete := false
	for range maxRecoveryPages {
		messages, err := t.rest.GetMessages(board.ChannelID, 0, before, 0, recoveryPageSize)
		if err != nil {
			return 0, fmt.Errorf("search starboard destination for nonce: %w", err)
		}
		oldest := snowflake.ID(^uint64(0))
		for _, message := range messages {
			if message.ID < oldest {
				oldest = message.ID
			}
			if message.ID >= startID && message.Author.ID == botID && (string(message.Nonce) == nonce || messageHasRecoveryMarker(message, nonce)) {
				return message.ID, nil
			}
		}
		if len(messages) < recoveryPageSize || oldest <= startID {
			complete = true
			break
		}
		if before != 0 && oldest >= before {
			break
		}
		before = oldest
	}
	if complete && time.Since(since) >= recoverySafetyDelay {
		canReadHistory, err := t.canReadDestinationHistory(board)
		if err != nil {
			return 0, err
		}
		if canReadHistory {
			return 0, ErrNotSent
		}
	}
	return 0, nil
}

const recoveryQueryParameter = "heimdallr_starboard"

func recoveryMarker(jumpURL, nonce string) string {
	parsed, err := url.Parse(jumpURL)
	if err != nil {
		return jumpURL
	}
	query := parsed.Query()
	query.Set(recoveryQueryParameter, nonce)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func messageHasRecoveryMarker(message discord.Message, nonce string) bool {
	for _, embed := range message.Embeds {
		parsed, err := url.Parse(embed.URL)
		if err == nil && parsed.Query().Get(recoveryQueryParameter) == nonce {
			return true
		}
	}
	return false
}

func (t *discordTransport) canReadDestinationHistory(board model.Starboard) (bool, error) {
	channel, err := t.rest.GetChannel(board.ChannelID)
	if err != nil {
		return false, fmt.Errorf("verify starboard destination history access: %w", err)
	}
	guildChannel, ok := channel.(discord.GuildChannel)
	if !ok || guildChannel.GuildID() != board.GuildID {
		return false, nil
	}
	guild, err := t.rest.GetGuild(board.GuildID, false)
	if err != nil {
		return false, fmt.Errorf("verify starboard guild history access: %w", err)
	}
	member, err := t.rest.GetMember(board.GuildID, t.selfID())
	if err != nil {
		return false, fmt.Errorf("verify bot history access: %w", err)
	}
	permissions := memberPermissions(guild, member, guildChannel)
	return permissions.Has(discord.PermissionViewChannel, discord.PermissionReadMessageHistory), nil
}

func (t *discordTransport) boardEmoji(board model.Starboard) (resolvedEmoji, error) {
	if !strings.ContainsRune(board.Emoji, ':') {
		return t.resolveEmoji(board.GuildID, board.Emoji, nil)
	}
	botID := t.selfID()
	if botID == 0 {
		return resolvedEmoji{}, fmt.Errorf("cannot determine bot user ID")
	}
	member, err := t.rest.GetMember(board.GuildID, botID)
	if err != nil {
		return resolvedEmoji{}, fmt.Errorf("fetch bot member: %w", err)
	}
	return t.resolveEmoji(board.GuildID, board.Emoji, member)
}

func (t *discordTransport) starboardPresentation(board model.Starboard, source *discord.Message, votes Votes, emojiDisplay string) (discord.Embed, []discord.LayoutComponent) {
	embed := t.quoteEmbed(source)
	embed = embed.AddField(
		"Score",
		fmt.Sprintf("%s %d  •  ❌ %d  •  **Net %d**", emojiDisplay, len(votes.Positive), len(votes.Negative), len(votes.Positive)-len(votes.Negative)),
		false,
	)
	embed = quote.BoundMessageQuoteEmbed(embed)
	jumpURL := discord.MessageURL(board.GuildID, source.ChannelID, source.ID)
	components := []discord.LayoutComponent{
		discord.NewActionRow(discord.NewLinkButton("Jump to message", jumpURL)),
	}
	return embed, components
}

func (t *discordTransport) seedReactions(channel, message snowflake.ID, positiveEmoji string) error {
	if err := t.rest.AddReaction(channel, message, positiveEmoji); err != nil {
		return fmt.Errorf("seed positive starboard reaction: %w", err)
	}
	if err := t.rest.AddReaction(channel, message, "❌"); err != nil {
		return fmt.Errorf("seed negative starboard reaction: %w", err)
	}
	return nil
}

func definitelyNotSent(err error) bool {
	var restErr *rest.Error
	if !errors.As(err, &restErr) || restErr.Response == nil {
		return false
	}
	status := restErr.Response.StatusCode
	return status >= http.StatusBadRequest && status < http.StatusInternalServerError && status != http.StatusRequestTimeout
}

func (t *discordTransport) Votes(channel, message snowflake.ID, emoji string) (Votes, error) {
	positive, err := t.reactionUsers(channel, message, emoji)
	if err != nil {
		return Votes{}, err
	}
	negative, err := t.reactionUsers(channel, message, "❌")
	if err != nil {
		return Votes{}, err
	}
	return Votes{Positive: positive, Negative: negative}, nil
}

func (t *discordTransport) reactionUsers(channel, message snowflake.ID, emoji string) ([]snowflake.ID, error) {
	users := make(map[snowflake.ID]struct{})
	for _, reactionType := range []discord.MessageReactionType{
		discord.MessageReactionTypeNormal,
		discord.MessageReactionTypeBurst,
	} {
		after := 0
		for {
			page, err := t.rest.GetReactions(channel, message, emoji, reactionType, after, reactionPageSize)
			if err != nil {
				if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage) {
					return nil, ErrNotFound
				}
				return nil, err
			}
			for _, user := range page {
				if !user.Bot {
					users[user.ID] = struct{}{}
				}
			}
			if len(page) < reactionPageSize {
				break
			}
			after = int(page[len(page)-1].ID)
		}
	}

	result := make([]snowflake.ID, 0, len(users))
	for userID := range users {
		result = append(result, userID)
	}
	return result, nil
}

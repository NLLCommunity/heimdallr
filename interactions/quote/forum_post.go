package quote

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

const (
	forumPostTitleInput               = "title"
	forumPostBodyInput                = "body"
	forumPostForumInput               = "forum"
	maxForumPostModalBodyRunes        = 2000
	maxForumPostEmbedDescriptionRunes = 4096
)

var CreateForumPostCommand = rave.MessageCommand("Copy to New Forum Thread").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionSendMessages).
	Handle(CreateForumPostHandler)

// CreateForumPostHandler starts a modal from the message context menu. The
// source message IDs travel in the modal custom ID; the message itself is
// fetched again on submit so permissions and authorship cannot be forged.
func CreateForumPostHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("create-forum-post", e)

	if e.GuildID() == nil {
		return interactions.ErrEventNoGuildID
	}

	message := e.MessageCommandInteractionData().TargetMessage()
	if canRead, err := userCanReadChannelMessages(e.User().ID, message.ChannelID, e.Client()); err != nil || !canRead {
		return e.CreateMessage(interactions.EphemeralMessageContent("You don't have permission to read this message."))
	}

	customID, err := forumPostModalRoute.CustomID(forumPostModalVars{
		ChannelID: message.ChannelID,
		MessageID: message.ID,
	})
	if err != nil {
		return err
	}
	return e.Modal(createForumPostModal(customID, &message, e.User().ID))
}

func createForumPostModal(customID string, message *discord.Message, userID snowflake.ID) discord.ModalCreate {
	modal := discord.NewModalCreate(customID, "Create Forum Post", nil).
		AddLabel(
			"Forum", discord.NewChannelSelectMenu(forumPostForumInput, "Select a forum").
				WithChannelTypes(discord.ChannelTypeGuildForum).
				WithRequired(true),
		).
		AddLabel(
			"Title", discord.NewShortTextInput(forumPostTitleInput).
				WithPlaceholder("Enter a suitable title for the new forum thread").
				WithMaxLength(100).
				WithRequired(true),
		)

	if message.Author.ID == userID && forumPostBodyFitsModal(message.Content) {
		modal = modal.AddLabel(
			"Message", discord.NewParagraphTextInput(forumPostBodyInput).
				WithValue(message.Content).
				WithMaxLength(maxForumPostModalBodyRunes),
		)
	} else {
		shortenedBody := message.Content
		if utf8.RuneCountInString(shortenedBody) > maxForumPostModalBodyRunes-3 {
			shortenedBody = shortenedBody[:maxForumPostModalBodyRunes-3] + "..."
		}
		modal = modal.AddComponents(discord.NewTextDisplay(
			"### Message\n-# This message cannot be edited because it is too long or you are not the author. The full message will be copied to the forum post.\n\n" +
				shortenedBody,
		))
	}

	return modal
}

func CreateForumPostModalHandler(e *handler.ModalEvent) error {
	_ = e.DeferCreateMessage(true)

	if e.GuildID() == nil {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("This action can only be used in a server."))
	}

	channelID, err := snowflake.Parse(e.Vars["channelID"])
	if err != nil {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("The source message is invalid."))
	}
	messageID, err := snowflake.Parse(e.Vars["messageID"])
	if err != nil {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("The source message is invalid."))
	}

	message, err := e.Client().Rest.GetMessage(channelID, messageID)
	if err != nil {
		slog.Warn("Failed to retrieve source message for forum post", "channel_id", channelID, "message_id", messageID, "err", err)
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("The source message could not be retrieved."))
	}
	if canRead, err := userCanReadChannelMessages(e.User().ID, message.ChannelID, e.Client()); err != nil || !canRead {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("You don't have permission to read this message."))
	}

	forums := e.Data.Channels(forumPostForumInput)
	if len(forums) != 1 || forums[0].Type != discord.ChannelTypeGuildForum {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("Select a forum channel."))
	}
	forum := forums[0]
	if forum.Permissions.Missing(
		discord.PermissionViewChannel,
		discord.PermissionCreatePublicThreads,
		discord.PermissionSendMessagesInThreads,
	) {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("You don't have permission to create posts in that forum."))
	}

	title := strings.TrimSpace(e.Data.Text(forumPostTitleInput))
	if title == "" || utf8.RuneCountInString(title) > 100 {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("Enter a forum post title of up to 100 characters."))
	}

	body := message.Content
	if message.Author.ID == e.User().ID {
		// Oversized source messages deliberately have no body field in the
		// modal, so keep the fetched source unless an editable value exists.
		if editedBody, ok := e.Data.OptText(forumPostBodyInput); ok {
			body = editedBody
		}
	}
	if forumPostBodyTooLong(body) {
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("The forum post message is too long."))
	}

	post, err := e.Client().Rest.CreatePostInThreadChannel(
		forum.ID,
		discord.ThreadChannelPostCreate{
			Name: title,
			Message: discord.NewMessageCreate().
				WithContent(forumPostAttribution(message, e.User())).
				WithEmbeds(forumPostEmbed(e.Client(), message, body)).
				AddActionRow(discord.NewLinkButton("View original message", message.JumpURL())).
				WithAllowedMentions(&discord.AllowedMentions{}),
		},
	)
	if err != nil {
		slog.Warn("Failed to create forum post from message", "forum_id", forum.ID, "message_id", message.ID, "err", err)
		return createForumPostFollowup(e, interactions.EphemeralMessageContent("Failed to create the forum post."))
	}

	if _, err := e.Client().Rest.CreateMessage(
		message.ChannelID,
		discord.NewMessageCreate().
			WithContent(sourceCopyNotice(forum.ID, post.Message.JumpURL(), message.Author, e.User())).
			WithMessageReference(&discord.MessageReference{
				MessageID:       &message.ID,
				ChannelID:       &message.ChannelID,
				GuildID:         e.GuildID(),
				FailIfNotExists: false,
			}).
			WithAllowedMentions(&discord.AllowedMentions{}),
	); err != nil {
		slog.Warn("Created forum post but failed to reply to source message", "forum_id", forum.ID, "message_id", message.ID, "err", err)
	}

	return createForumPostFollowup(e,
		interactions.EphemeralMessageContent("Forum post created.").
			AddActionRow(discord.NewLinkButton("View forum post", post.Message.JumpURL())),
	)
}

func createForumPostFollowup(e *handler.ModalEvent, message discord.MessageCreate) error {
	_, err := e.CreateFollowupMessage(message)
	return err
}

func forumPostBodyTooLong(body string) bool {
	return utf8.RuneCountInString(body) > maxForumPostEmbedDescriptionRunes
}

func forumPostBodyFitsModal(body string) bool {
	return utf8.RuneCountInString(body) <= maxForumPostModalBodyRunes
}

func forumPostEmbed(client *bot.Client, message *discord.Message, body string) discord.Embed {
	embed := CreateMessageQuoteEmbed(client, message, false)
	embed.Title = "Copied message"
	embed.Description = body
	return embed
}

func forumPostAttribution(message *discord.Message, copier discord.User) string {
	if message.Author.ID == copier.ID {
		return fmt.Sprintf("Posted by the bot on behalf of %s, copied from the [original message](%s).", message.Author.Mention(), message.JumpURL())
	}
	return fmt.Sprintf("Posted by the bot on behalf of %s, copied from the [original message](%s) by %s.", message.Author.Mention(), message.JumpURL(), copier.Mention())
}

func sourceCopyNotice(forumID snowflake.ID, postURL string, author, copier discord.User) string {
	notice := fmt.Sprintf("Copied to <#%s> as [a forum post](%s).", forumID, postURL)
	if author.ID != copier.ID {
		notice += fmt.Sprintf(" Copied by %s.", copier.Mention())
	}
	return notice
}

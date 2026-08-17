package quote

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterInstallsForumPostModalRoute(t *testing.T) {
	router := handler.New()
	Register(router)
	require.True(t, router.Match("/quote/forum-post/2/3", discord.InteractionTypeModalSubmit, 0))
}

func TestForumPostModalRouteID(t *testing.T) {
	id, err := forumPostModalRoute.CustomID(forumPostModalVars{ChannelID: 2, MessageID: 3})
	require.NoError(t, err)
	require.Equal(t, "/quote/forum-post/2/3", id)
}

func TestForumPostBodyTooLongUsesCharacterCount(t *testing.T) {
	assert.False(t, forumPostBodyTooLong(strings.Repeat("å", maxForumPostEmbedDescriptionRunes)))
	assert.True(t, forumPostBodyTooLong(strings.Repeat("å", maxForumPostEmbedDescriptionRunes+1)))
}

func TestForumPostBodyFitsModal(t *testing.T) {
	assert.True(t, forumPostBodyFitsModal(strings.Repeat("x", maxForumPostModalBodyRunes)))
	assert.False(t, forumPostBodyFitsModal(strings.Repeat("x", maxForumPostModalBodyRunes+1)))
}

func TestForumPostAttribution(t *testing.T) {
	guildID := snowflake.ID(1)
	message := &discord.Message{
		ID:        3,
		GuildID:   &guildID,
		ChannelID: 2,
		Author:    discord.User{ID: 4},
	}

	assert.Contains(t, forumPostAttribution(message, discord.User{ID: 4}), "on behalf of <@4>")
	assert.NotContains(t, forumPostAttribution(message, discord.User{ID: 4}), "by <@4>")
	assert.Contains(t, forumPostAttribution(message, discord.User{ID: 5}), "by <@5>")
}

func TestCreateForumPostModalOnlyLetsAuthorEditMessage(t *testing.T) {
	message := &discord.Message{
		Content: "Source text",
		Author:  discord.User{ID: 4},
	}

	authorModal := createForumPostModal("/quote/forum-post/2/3", message, 4)
	otherUserModal := createForumPostModal("/quote/forum-post/2/3", message, 5)

	assert.Len(t, authorModal.Components, 3)
	assert.Len(t, otherUserModal.Components, 3)
	assert.Equal(t, authorModal.Components[len(authorModal.Components)-1].Type(), discord.ComponentTypeLabel)
	assert.Equal(t, otherUserModal.Components[len(otherUserModal.Components)-1].Type(), discord.ComponentTypeTextDisplay)

	authorMessageField := authorModal.Components[2].(discord.LabelComponent)
	assert.Equal(t, "Message", authorMessageField.Label)
	bodyInput := authorMessageField.Component.(discord.TextInputComponent)
	assert.Equal(t, forumPostBodyInput, bodyInput.CustomID)

	longMessage := &discord.Message{
		Content: strings.Repeat("x", maxForumPostModalBodyRunes+1),
		Author:  discord.User{ID: 4},
	}
	assert.Len(t, createForumPostModal("/quote/forum-post/2/3", longMessage, 4).Components, 3)
}

func TestSourceCopyNotice(t *testing.T) {
	author := discord.User{ID: 4}
	assert.NotContains(t, sourceCopyNotice(9, "https://example.test/post", author, author), "Copied by")
	assert.Contains(t, sourceCopyNotice(9, "https://example.test/post", author, discord.User{ID: 5}), "Copied by <@5>")
}

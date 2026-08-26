package quote

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/stretchr/testify/require"
)

type failingRESTClient struct{}

func (*failingRESTClient) HTTPClient() *http.Client { return nil }

func (*failingRESTClient) RateLimiter() rest.RateLimiter { return nil }

func (*failingRESTClient) Close(context.Context) {}

func (*failingRESTClient) Do(*rest.CompiledEndpoint, any, any, ...rest.RequestOpt) error {
	return errors.New("unavailable")
}

func quoteTestClient() *bot.Client {
	return &bot.Client{Rest: rest.New(&failingRESTClient{})}
}

func TestQuoteCommandUsesDefaultIntegrationTypesInGuildContext(t *testing.T) {
	command, ok := Quote.Build().(discord.SlashCommandCreate)
	require.True(t, ok)
	require.Nil(t, command.IntegrationTypes)
	require.Equal(t, []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	}, command.Contexts)
}

func TestCreateMessageQuoteEmbedIncludesSingleAttachmentImage(t *testing.T) {
	embed := CreateMessageQuoteEmbed(quoteTestClient(), &discord.Message{
		Content: "Look at this",
		Author:  discord.User{Username: "author"},
		Attachments: []discord.Attachment{
			{Filename: "image.png", URL: "https://cdn.example/image.png"},
		},
	}, false)

	require.NotNil(t, embed.Image)
	require.Equal(t, "https://cdn.example/image.png", embed.Image.URL)
}

func TestCreateMessageQuoteEmbedIncludesAttachmentAndReplyFields(t *testing.T) {
	embed := CreateMessageQuoteEmbed(quoteTestClient(), &discord.Message{
		Author: discord.User{Username: "author"},
		Attachments: []discord.Attachment{
			{Filename: "one.png", URL: "https://cdn.example/one.png"},
			{Filename: "two.png", URL: "https://cdn.example/two.png"},
		},
		ReferencedMessage: &discord.Message{
			ID:        42,
			ChannelID: 24,
			Content:   "Original message",
			Author:    discord.User{ID: 7, Username: "original"},
		},
	}, true)

	require.Len(t, embed.Fields, 2)
	require.Equal(t, "Attachments", embed.Fields[0].Name)
	require.Equal(t, "- [one.png](https://cdn.example/one.png)\n- [two.png](https://cdn.example/two.png)", embed.Fields[0].Value)
	require.Equal(t, "Reply to", embed.Fields[1].Name)
	require.Contains(t, embed.Fields[1].Value, "Original message")
}

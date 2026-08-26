package posts

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageUpdateRecorder struct {
	update discord.MessageUpdate
}

func (*messageUpdateRecorder) HTTPClient() *http.Client {
	return nil
}

func (*messageUpdateRecorder) RateLimiter() rest.RateLimiter {
	return nil
}

func (*messageUpdateRecorder) Close(context.Context) {}

func (r *messageUpdateRecorder) Do(
	_ *rest.CompiledEndpoint,
	input any,
	_ any,
	_ ...rest.RequestOpt,
) error {
	update, ok := input.(discord.MessageUpdate)
	if !ok {
		return fmt.Errorf("expected discord.MessageUpdate, got %T", input)
	}
	r.update = update
	return nil
}

func TestLiveDiscordEditV2SendsEveryComponentWithoutMentions(t *testing.T) {
	recorder := &messageUpdateRecorder{}
	client := &bot.Client{Rest: rest.New(recorder)}
	discordClient := &liveDiscord{client: client}

	err := discordClient.EditV2(11, 22, []any{
		map[string]any{"type": float64(typeTextDisplay), "content": "first"},
		map[string]any{"type": float64(typeTextDisplay), "content": "second"},
	})
	require.NoError(t, err)

	require.NotNil(t, recorder.update.Components)
	require.Len(t, *recorder.update.Components, 2)
	first, ok := (*recorder.update.Components)[0].(discord.TextDisplayComponent)
	require.True(t, ok)
	second, ok := (*recorder.update.Components)[1].(discord.TextDisplayComponent)
	require.True(t, ok)
	assert.Equal(t, "first", first.Content)
	assert.Equal(t, "second", second.Content)

	require.NotNil(t, recorder.update.Flags)
	assert.True(t, recorder.update.Flags.Has(discord.MessageFlagIsComponentsV2))
	assert.Equal(t, &discord.AllowedMentions{}, recorder.update.AllowedMentions)
}

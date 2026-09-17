package pages

import (
	"context"
	"strings"
	"testing"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStarboardsShowsStoredMissingDestination(t *testing.T) {
	var output strings.Builder
	err := Starboards(layouts.NavData{}, StarboardsData{
		GuildID: "100",
		Boards:  []model.Starboard{{ID: 1, GuildID: 100, ChannelID: 999, Name: "Old board", Emoji: "⭐", Threshold: 4}},
	}).Render(context.Background(), &output)
	require.NoError(t, err)

	html := output.String()
	assert.Contains(t, html, `value="999" selected`)
	assert.Contains(t, html, `Unavailable channel (999)`)
}

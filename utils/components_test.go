package utils

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Section without accessory should be flattened into its child text_displays.
// (Section type = 9; TextDisplay = 10; Container = 17.)
func TestParseComponents_SectionWithoutAccessory_IsFlattened(t *testing.T) {
	json := `[
		{"type": 17, "components": [
			{"type": 9, "components": [
				{"type": 10, "content": "hello"},
				{"type": 10, "content": "world"}
			]}
		]}
	]`
	out, err := ParseComponents(json)
	require.NoError(t, err)
	require.Len(t, out, 1)
	container, ok := out[0].(discord.ContainerComponent)
	require.True(t, ok, "expected ContainerComponent, got %T", out[0])
	require.Len(t, container.Components, 2)
	for i, c := range container.Components {
		td, ok := c.(discord.TextDisplayComponent)
		require.True(t, ok, "child %d: want TextDisplay, got %T", i, c)
		assert.NotEmpty(t, td.Content)
	}
}

// "accessory": null must be treated the same as a missing accessory — flatten,
// don't pass through (disgo's section unmarshaler panics on nil accessory).
func TestParseComponents_SectionWithNullAccessory_IsFlattened(t *testing.T) {
	json := `[
		{"type": 17, "components": [
			{"type": 9, "accessory": null, "components": [
				{"type": 10, "content": "hello"}
			]}
		]}
	]`
	out, err := ParseComponents(json)
	require.NoError(t, err)
	require.Len(t, out, 1)
	container, ok := out[0].(discord.ContainerComponent)
	require.True(t, ok)
	require.Len(t, container.Components, 1)
	_, ok = container.Components[0].(discord.TextDisplayComponent)
	assert.True(t, ok, "want TextDisplay after flatten, got %T", container.Components[0])
}

// Emoji resolution: an emoji object with a name but no id should be replaced
// with the matching entry from the emoji map.
func TestResolveEmojis_ResolvesNamedEmojiWithoutID(t *testing.T) {
	emojis := map[string]discord.Emoji{
		"thumbsup": {ID: 12345, Name: "thumbsup"},
	}
	tree := map[string]any{
		"emoji": map[string]any{"name": ":thumbsup:"},
	}
	require.NoError(t, ResolveEmojis(tree, emojis))
	got := tree["emoji"].(map[string]any)
	assert.Equal(t, "thumbsup", got["name"])
	assert.Equal(t, "12345", got["id"])
}

// Already-resolved emoji (has id) should be left alone.
func TestResolveEmojis_LeavesEmojiWithIDAlone(t *testing.T) {
	emojis := map[string]discord.Emoji{
		"thumbsup": {ID: 99999, Name: "thumbsup"},
	}
	tree := map[string]any{
		"emoji": map[string]any{"name": "custom", "id": "555"},
	}
	require.NoError(t, ResolveEmojis(tree, emojis))
	got := tree["emoji"].(map[string]any)
	assert.Equal(t, "custom", got["name"])
	assert.Equal(t, "555", got["id"])
}

// Unknown emoji name: leave the object unchanged.
func TestResolveEmojis_UnknownNameUnchanged(t *testing.T) {
	emojis := map[string]discord.Emoji{}
	tree := map[string]any{
		"emoji": map[string]any{"name": "unknown"},
	}
	require.NoError(t, ResolveEmojis(tree, emojis))
	got := tree["emoji"].(map[string]any)
	assert.Equal(t, "unknown", got["name"])
	_, hasID := got["id"]
	assert.False(t, hasID)
}

package utils

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
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

// BuildV2MessageNoTemplate is the sandbox's send-time validator + parser. The
// save-flow validator (validateAndCompactV2JSON) used to enforce these
// invariants before the recent JSON-cycle refactor; the same guards now live
// in the shared parseComponentsFromAny core, so the sandbox can't slip an
// empty/non-array payload through to Discord.

func TestBuildV2MessageNoTemplate_RejectsTopLevelObject(t *testing.T) {
	_, err := BuildV2MessageNoTemplate(`{"type":10,"content":"hi"}`, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errNotTopLevelArray)
}

func TestBuildV2MessageNoTemplate_RejectsEmptyArray(t *testing.T) {
	_, err := BuildV2MessageNoTemplate(`[]`, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errEmptyComponents)
}

func TestBuildV2MessageNoTemplate_RejectsMalformedJSON(t *testing.T) {
	// Truncated input — must surface as an error rather than panicking or
	// silently producing an empty component slice.
	_, err := BuildV2MessageNoTemplate(`[{"type":10`, nil)
	assert.Error(t, err)
}

// Sanity check for the success path: a well-formed top-level array with one
// text_display round-trips into a non-empty []discord.LayoutComponent.
func TestBuildV2MessageNoTemplate_ValidComponents(t *testing.T) {
	out, err := BuildV2MessageNoTemplate(`[{"type":10,"content":"hi"}]`, nil)
	require.NoError(t, err)
	require.Len(t, out, 1)
	td, ok := out[0].(discord.TextDisplayComponent)
	require.True(t, ok, "want TextDisplayComponent, got %T", out[0])
	assert.Equal(t, "hi", td.Content)
}

// Emoji resolution still happens for sandbox sends: a button with a named
// emoji but no ID gets the ID filled in from the guild's emoji map.
func TestBuildV2MessageNoTemplate_ResolvesEmoji(t *testing.T) {
	emojis := map[string]discord.Emoji{
		"thumbsup": {ID: 12345, Name: "thumbsup"},
	}
	json := `[{"type":1,"components":[
		{"type":2,"style":5,"label":"go","url":"https://example.com","emoji":{"name":":thumbsup:"}}
	]}]`
	out, err := BuildV2MessageNoTemplate(json, emojis)
	require.NoError(t, err)
	require.Len(t, out, 1)
	row, ok := out[0].(discord.ActionRowComponent)
	require.True(t, ok, "want ActionRowComponent, got %T", out[0])
	require.Len(t, row.Components, 1)
	btn, ok := row.Components[0].(discord.ButtonComponent)
	require.True(t, ok, "want ButtonComponent, got %T", row.Components[0])
	require.NotNil(t, btn.Emoji)
	assert.Equal(t, "thumbsup", btn.Emoji.Name)
	assert.Equal(t, snowflake.ID(12345), btn.Emoji.ID)
}

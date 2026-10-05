package posts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDocument_LegacyArrayUsesPlanBoundaries(t *testing.T) {
	half := strings.Repeat("x", maxTextDisplayCharsTotal/2+1)
	raw := `[{"type":10,"content":"` + half + `"},{"type":10,"content":"` + half + `"}]`

	doc, err := ParseDocument(raw)
	require.NoError(t, err)
	require.Len(t, doc.Messages, 2)
	assert.Len(t, doc.Messages[0].Components, 1)
	assert.Len(t, doc.Messages[1].Components, 1)

	canonical, err := doc.JSON()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(canonical, `{"version":1,"messages":[`))
}

func TestParseDocument_PreservesExplicitMessageBoundaries(t *testing.T) {
	raw := `{"version":1,"messages":[{"components":[{"type":10,"content":"a"}]},{"components":[{"type":10,"content":"b"}]}]}`

	doc, err := ParseDocument(raw)
	require.NoError(t, err)
	require.Len(t, doc.Messages, 2)
	assert.Equal(t, "a", doc.Messages[0].Components[0].(map[string]any)["content"])
	assert.Equal(t, "b", doc.Messages[1].Components[0].(map[string]any)["content"])
}

func TestParseDocument_RejectsExplicitMessageThatWouldNeedSplitting(t *testing.T) {
	half := strings.Repeat("x", maxTextDisplayCharsTotal/2+1)
	raw := `{"version":1,"messages":[{"components":[{"type":10,"content":"` + half + `"},{"type":10,"content":"` + half + `"}]}]}`

	_, err := ParseDocument(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message 1")
	assert.Contains(t, err.Error(), "single Discord message")
}

func TestParseDocument_AllowsEmptyDraftMessages(t *testing.T) {
	doc, err := ParseDocument(`{"version":1,"messages":[{"components":[]}]}`)
	require.NoError(t, err)
	assert.Len(t, doc.Messages, 1)
	assert.Empty(t, doc.Messages[0].Components)
}

func TestDocumentPublishChunksRejectsEmptyMessagesAndZeroMessageDocument(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"messages":[]}`,
		`{"version":1,"messages":[{"components":[]}]}`,
	} {
		doc, err := ParseDocument(raw)
		require.NoError(t, err)
		_, err = doc.PublishChunks()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no content")
	}
}

func TestDocumentPublishChunksPreservesExplicitBoundaries(t *testing.T) {
	doc, err := ParseDocument(`{"version":1,"messages":[{"components":[{"type":10,"content":"a"}]},{"components":[{"type":10,"content":"b"}]}]}`)
	require.NoError(t, err)

	chunks, err := doc.PublishChunks()
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	assert.Equal(t, "a", chunks[0][0].(map[string]any)["content"])
	assert.Equal(t, "b", chunks[1][0].(map[string]any)["content"])
}

func TestParseDocument_RejectsUnsupportedOrLossyShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"unsupported version", `{"version":2,"messages":[]}`},
		{"unknown document field", `{"version":1,"messages":[],"future":true}`},
		{"unknown message field", `{"version":1,"messages":[{"components":[],"future":true}]}`},
		{"missing messages", `{"version":1}`},
		{"missing components", `{"version":1,"messages":[{}]}`},
		{"null messages", `{"version":1,"messages":null}`},
		{"null components", `{"version":1,"messages":[{"components":null}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDocument(tc.raw)
			assert.Error(t, err)
		})
	}
}

func TestParseDocument_RejectsDiscordSchemaMismatchPerMessage(t *testing.T) {
	_, err := ParseDocument(`{"version":1,"messages":[{"components":[{"type":10,"content":42}]}]}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Discord schema check")
}

func TestParseDocument_ValidationDoesNotRewriteAccessorylessSection(t *testing.T) {
	raw := `{"version":1,"messages":[{"components":[{"type":17,"components":[{"type":9,"components":[{"type":10,"content":"keep section"}]}]}]}]}`

	doc, err := ParseDocument(raw)
	require.NoError(t, err)
	canonical, err := doc.JSON()
	require.NoError(t, err)
	assert.JSONEq(t, raw, canonical)
}

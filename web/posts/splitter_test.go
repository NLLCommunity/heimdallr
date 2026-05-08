package posts

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func parseAny(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("invalid test JSON: %v", err)
	}
	return v
}

func TestComponentCount_LeafIsOne(t *testing.T) {
	tx := parseAny(t, `{"type":10,"content":"hello"}`)
	assert.Equal(t, 1, componentCount(tx))
}

func TestComponentCount_ContainerCountsItself(t *testing.T) {
	c := parseAny(t, `{"type":17,"components":[{"type":10,"content":"a"},{"type":10,"content":"b"}]}`)
	assert.Equal(t, 3, componentCount(c))
}

func TestComponentCount_SectionWithAccessory(t *testing.T) {
	s := parseAny(t, `{"type":9,"components":[{"type":10,"content":"x"}],"accessory":{"type":11,"media":{"url":"http://e.com/a.png"}}}`)
	assert.Equal(t, 3, componentCount(s))
}

func TestTextDisplayCharCount_TextDisplay(t *testing.T) {
	tx := parseAny(t, `{"type":10,"content":"hello world"}`)
	assert.Equal(t, 11, textDisplayCharCount(tx))
}

func TestTextDisplayCharCount_NonTextIsZero(t *testing.T) {
	sep := parseAny(t, `{"type":14,"divider":true}`)
	assert.Equal(t, 0, textDisplayCharCount(sep))
}

func TestTextDisplayCharCount_RecursesIntoContainer(t *testing.T) {
	c := parseAny(t, `{"type":17,"components":[{"type":10,"content":"abc"},{"type":10,"content":"de"}]}`)
	assert.Equal(t, 5, textDisplayCharCount(c))
}

func TestTextDisplayCharCount_RecursesIntoSection(t *testing.T) {
	s := parseAny(t, `{"type":9,"components":[{"type":10,"content":"hello"}],"accessory":{"type":11,"media":{"url":"http://e.com/a.png"}}}`)
	assert.Equal(t, 5, textDisplayCharCount(s))
}

func TestMediaItemCount_GalleryReturnsLen(t *testing.T) {
	g := parseAny(t, `{"type":12,"items":[{"media":{"url":"http://e.com/1.png"}},{"media":{"url":"http://e.com/2.png"}}]}`)
	assert.Equal(t, 2, mediaItemCount(g))
}

func TestMediaItemCount_TextDisplayIsZero(t *testing.T) {
	tx := parseAny(t, `{"type":10,"content":"hi"}`)
	assert.Equal(t, 0, mediaItemCount(tx))
}

func TestMediaItemCount_RecursesIntoContainer(t *testing.T) {
	c := parseAny(t, `{"type":17,"components":[{"type":12,"items":[{"media":{"url":"http://e.com/a.png"}}]}]}`)
	assert.Equal(t, 1, mediaItemCount(c))
}

func TestValidateComponent_OkForSmallText(t *testing.T) {
	tx := parseAny(t, `{"type":10,"content":"short"}`)
	assert.NoError(t, validateComponent(tx))
}

func TestValidateComponent_RejectsOversizedTextDisplay(t *testing.T) {
	big := make([]byte, maxTextDisplayCharsTotal+1)
	for i := range big {
		big[i] = 'x'
	}
	tx := parseAny(t, `{"type":10,"content":"`+string(big)+`"}`)
	err := validateComponent(tx)
	assert.Error(t, err)
}

func TestValidateComponent_RejectsMissingType(t *testing.T) {
	tx := parseAny(t, `{"content":"hi"}`)
	err := validateComponent(tx)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "type")
	}
}

func TestValidateComponent_RejectsUnknownType(t *testing.T) {
	tx := parseAny(t, `{"type":999,"content":"hi"}`)
	err := validateComponent(tx)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "999")
	}
}

func TestValidateComponent_RejectsTopLevelButton(t *testing.T) {
	// Buttons are only valid nested in an action_row or as a section
	// accessory. A top-level button would be silently dropped by disgo at
	// publish; reject at save instead.
	tx := parseAny(t, `{"type":2,"label":"x","style":5,"url":"https://e.com"}`)
	err := validateComponent(tx)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "top-level")
	}
}

func TestValidateComponent_RejectsNestedUnknownType(t *testing.T) {
	c := parseAny(t, `{"type":17,"components":[{"type":42,"content":"weird"}]}`)
	err := validateComponent(c)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "42")
	}
}

func TestValidateComponent_RejectsUnknownTypeNestedInAccessory(t *testing.T) {
	// A malformed accessory carrying its own "components" array with an
	// unknown nested type must still be rejected — walkNestedTypes recurses
	// into accessories.
	s := parseAny(t, `{"type":9,"components":[{"type":10,"content":"x"}],"accessory":{"type":11,"components":[{"type":42,"content":"weird"}]}}`)
	err := validateComponent(s)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "42")
	}
}

func TestValidateComponent_RejectsFractionalType(t *testing.T) {
	// 10.9 must not be silently truncated to 10 (text_display).
	tx := parseAny(t, `{"type":10.9,"content":"hi"}`)
	err := validateComponent(tx)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "integer")
	}
}

func TestValidateComponent_RejectsFractionalNestedType(t *testing.T) {
	c := parseAny(t, `{"type":17,"components":[{"type":10.5,"content":"hi"}]}`)
	err := validateComponent(c)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "integer")
	}
}

func TestValidateComponent_RejectsTooManyMediaInGallery(t *testing.T) {
	items := make([]map[string]any, maxMediaItemsTotal+1)
	for i := range items {
		items[i] = map[string]any{"media": map[string]any{"url": "http://e.com/x.png"}}
	}
	g := map[string]any{"type": float64(typeMediaGallery), "items": toAnySlice(items)}
	err := validateComponent(g)
	assert.Error(t, err)
}

// toAnySlice converts a typed slice to []any for parity with json.Unmarshal output.
func toAnySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func TestPlan_EmptyInputReturnsEmpty(t *testing.T) {
	chunks, err := Plan(nil)
	assert.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestPlan_SingleSmallComponentFitsInOneMessage(t *testing.T) {
	in := []any{parseAny(t, `{"type":10,"content":"hi"}`)}
	chunks, err := Plan(in)
	assert.NoError(t, err)
	assert.Len(t, chunks, 1)
	assert.Len(t, chunks[0], 1)
}

func TestPlan_PacksMultipleComponentsIntoOneMessage(t *testing.T) {
	in := []any{
		parseAny(t, `{"type":10,"content":"a"}`),
		parseAny(t, `{"type":10,"content":"b"}`),
		parseAny(t, `{"type":10,"content":"c"}`),
	}
	chunks, err := Plan(in)
	assert.NoError(t, err)
	assert.Len(t, chunks, 1)
	assert.Len(t, chunks[0], 3)
}

func TestPlan_SplitsOnCharLimit(t *testing.T) {
	half := make([]byte, maxTextDisplayCharsTotal/2+10)
	for i := range half {
		half[i] = 'x'
	}
	mk := func() any {
		return map[string]any{"type": float64(typeTextDisplay), "content": string(half)}
	}
	in := []any{mk(), mk(), mk()}
	chunks, err := Plan(in)
	assert.NoError(t, err)
	assert.Len(t, chunks, 3)
}

func TestPlan_SplitsOnComponentCountLimit(t *testing.T) {
	mk := func() any { return map[string]any{"type": float64(typeTextDisplay), "content": "a"} }
	in := make([]any, maxComponentsPerMessage+1)
	for i := range in {
		in[i] = mk()
	}
	chunks, err := Plan(in)
	assert.NoError(t, err)
	assert.Len(t, chunks, 2)
	assert.Len(t, chunks[0], maxComponentsPerMessage)
	assert.Len(t, chunks[1], 1)
}

func TestPlan_RejectsSingleOversizedComponent(t *testing.T) {
	big := make([]byte, maxTextDisplayCharsTotal+1)
	for i := range big {
		big[i] = 'x'
	}
	in := []any{map[string]any{"type": float64(typeTextDisplay), "content": string(big)}}
	_, err := Plan(in)
	assert.Error(t, err)
}

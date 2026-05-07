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

// Package posts contains the pure splitting algorithm that packs an array
// of V2 components into Discord-message-sized chunks, plus the sync engine
// that drives Discord-side updates from a Post's stored state.
package posts

import (
	"encoding/json"
	"fmt"
)

// Discord per-message caps for V2 component messages. Update if Discord
// publishes new limits.
const (
	maxComponentsPerMessage  = 40
	maxTextDisplayCharsTotal = 4000
	maxMediaItemsTotal       = 10
	maxTextDisplayCharsEach  = 4000 // single text_display max content length
)

// Discord component type codes (mirrors message-builder.js).
const (
	typeActionRow    = 1
	typeButton       = 2
	typeSection      = 9
	typeTextDisplay  = 10
	typeThumbnail    = 11
	typeMediaGallery = 12
	typeSeparator    = 14
	typeContainer    = 17
)

// componentCount returns the total number of components in a tree, counting
// the node itself plus all nested children, plus accessories on sections.
// Recurses into "components" arrays and "accessory" sub-objects.
func componentCount(v any) int {
	obj, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	count := 1
	if children, ok := obj["components"].([]any); ok {
		for _, c := range children {
			count += componentCount(c)
		}
	}
	if acc, ok := obj["accessory"].(map[string]any); ok {
		count += componentCount(acc)
	}
	return count
}

// textDisplayCharCount sums the rune length of every text_display "content"
// in the subtree. Used to enforce Discord's per-message character cap.
func textDisplayCharCount(v any) int {
	obj, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	total := 0
	if t, ok := obj["type"].(float64); ok && int(t) == typeTextDisplay {
		if s, ok := obj["content"].(string); ok {
			total += len([]rune(s))
		}
	}
	if children, ok := obj["components"].([]any); ok {
		for _, c := range children {
			total += textDisplayCharCount(c)
		}
	}
	if acc, ok := obj["accessory"].(map[string]any); ok {
		total += textDisplayCharCount(acc)
	}
	return total
}

// jsonMustMarshal returns the marshaled bytes or panics — used in test/
// rendering paths where the structure is known to be safe.
func jsonMustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// mediaItemCount sums the number of items inside any media_gallery in the
// subtree. Used to enforce Discord's per-message attachment cap.
func mediaItemCount(v any) int {
	obj, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	total := 0
	if t, ok := obj["type"].(float64); ok && int(t) == typeMediaGallery {
		if items, ok := obj["items"].([]any); ok {
			total += len(items)
		}
	}
	if children, ok := obj["components"].([]any); ok {
		for _, c := range children {
			total += mediaItemCount(c)
		}
	}
	if acc, ok := obj["accessory"].(map[string]any); ok {
		total += mediaItemCount(acc)
	}
	return total
}

// validateComponent rejects components that, in isolation, can never fit a
// Discord message — used both at save time (early feedback) and at split
// time (defense in depth). Callers that need full per-message limits enforced
// should check fits() instead.
func validateComponent(v any) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("component is not an object")
	}
	if textDisplayCharCount(obj) > maxTextDisplayCharsEach {
		return fmt.Errorf("text_display content exceeds %d characters", maxTextDisplayCharsEach)
	}
	if mediaItemCount(obj) > maxMediaItemsTotal {
		return fmt.Errorf("component contains more than %d media items", maxMediaItemsTotal)
	}
	if componentCount(obj) > maxComponentsPerMessage {
		return fmt.Errorf("component tree exceeds %d total components", maxComponentsPerMessage)
	}
	return nil
}

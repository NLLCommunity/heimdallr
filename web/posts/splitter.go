// Package posts contains the pure splitting algorithm that packs an array
// of V2 components into Discord-message-sized chunks, plus the sync engine
// that drives Discord-side updates from a Post's stored state.
package posts

import (
	"fmt"
	"unicode/utf8"
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

// componentTypeNames lists every V2 component type the splitter recognizes.
// Anything outside this set is rejected at save time so a malformed payload
// can't sit in the DB and only blow up at publish.
var componentTypeNames = map[int]string{
	typeActionRow:    "action_row",
	typeButton:       "button",
	typeSection:      "section",
	typeTextDisplay:  "text_display",
	typeThumbnail:    "thumbnail",
	typeMediaGallery: "media_gallery",
	typeSeparator:    "separator",
	typeContainer:    "container",
}

// topLevelComponentTypes is the subset of component types Discord allows at
// the top level of a V2 message. Buttons and thumbnails only appear nested
// (action_row children, section accessories), and rejecting them up-front is
// cheaper and clearer than letting disgo's parser drop them silently.
var topLevelComponentTypes = map[int]bool{
	typeActionRow:    true,
	typeSection:      true,
	typeTextDisplay:  true,
	typeMediaGallery: true,
	typeSeparator:    true,
	typeContainer:    true,
}

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
			total += utf8.RuneCountInString(s)
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

// validateStructure checks the basic V2-component shape so malformed input
// fails at save time instead of at publish:
//
//   - The root must be a JSON object with a numeric "type" field.
//   - That type must be valid at the top level (no bare buttons/thumbnails).
//   - Any nested object under "components" or as a section "accessory" must
//     have a known type code.
//
// Position-specific rules deeper in the tree (an action_row may only contain
// buttons; a section's accessory must be a thumbnail or button) are
// intentionally not enforced here — disgo's parser already rejects those at
// publish, and duplicating the rules here risks drift.
func validateStructure(v any) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("component is not a JSON object")
	}
	typeF, ok := obj["type"].(float64)
	if !ok {
		return fmt.Errorf(`component is missing a numeric "type" field`)
	}
	t := int(typeF)
	if !topLevelComponentTypes[t] {
		if name, known := componentTypeNames[t]; known {
			return fmt.Errorf("type %d (%s) is not valid as a top-level component", t, name)
		}
		return fmt.Errorf("unknown component type %d", t)
	}
	return walkNestedTypes(obj)
}

// walkNestedTypes recurses through "components" arrays and "accessory"
// objects, requiring each to carry a known type code. Used by validateStructure
// after the top-level check has succeeded.
func walkNestedTypes(obj map[string]any) error {
	if children, ok := obj["components"].([]any); ok {
		for _, c := range children {
			childObj, ok := c.(map[string]any)
			if !ok {
				return fmt.Errorf("nested component is not a JSON object")
			}
			if err := requireKnownType(childObj, "nested component"); err != nil {
				return err
			}
			if err := walkNestedTypes(childObj); err != nil {
				return err
			}
		}
	}
	if acc, ok := obj["accessory"].(map[string]any); ok {
		if err := requireKnownType(acc, "section accessory"); err != nil {
			return err
		}
	}
	return nil
}

func requireKnownType(obj map[string]any, label string) error {
	typeF, ok := obj["type"].(float64)
	if !ok {
		return fmt.Errorf(`%s is missing a numeric "type" field`, label)
	}
	if _, known := componentTypeNames[int(typeF)]; !known {
		return fmt.Errorf("unknown %s type %d", label, int(typeF))
	}
	return nil
}

// validateComponent rejects components that, in isolation, can never fit a
// Discord message — used both at save time (early feedback) and at split
// time (defense in depth). Callers that need full per-message limits enforced
// should check fits() instead.
func validateComponent(v any) error {
	if err := validateStructure(v); err != nil {
		return err
	}
	obj := v.(map[string]any)
	if textDisplayCharCount(obj) > maxTextDisplayCharsTotal {
		return fmt.Errorf("component contains more than %d total text_display characters", maxTextDisplayCharsTotal)
	}
	if mediaItemCount(obj) > maxMediaItemsTotal {
		return fmt.Errorf("component contains more than %d media items", maxMediaItemsTotal)
	}
	if componentCount(obj) > maxComponentsPerMessage {
		return fmt.Errorf("component tree exceeds %d total components", maxComponentsPerMessage)
	}
	return nil
}

// fits reports whether the candidate chunk (current + appendant) is within
// every per-message Discord limit. The "candidate" is conceptually the chunk
// you would have if you appended `c` to `current`.
func fits(current []any, c any) bool {
	totalCount := 0
	totalChars := 0
	totalMedia := 0
	for _, x := range current {
		totalCount += componentCount(x)
		totalChars += textDisplayCharCount(x)
		totalMedia += mediaItemCount(x)
	}
	totalCount += componentCount(c)
	totalChars += textDisplayCharCount(c)
	totalMedia += mediaItemCount(c)
	return totalCount <= maxComponentsPerMessage &&
		totalChars <= maxTextDisplayCharsTotal &&
		totalMedia <= maxMediaItemsTotal
}

// Plan packs a top-level component array into the smallest number of
// Discord-message-sized chunks, splitting only at top-level boundaries.
// Returns an error if a single component exceeds per-component caps and
// could never fit a message on its own.
func Plan(components []any) ([][]any, error) {
	for _, c := range components {
		if err := validateComponent(c); err != nil {
			return nil, err
		}
	}

	var out [][]any
	var current []any
	for _, c := range components {
		if !fits(current, c) {
			if len(current) == 0 {
				// Single component doesn't fit on its own — should have been
				// caught by validateComponent above, but treat defensively.
				return nil, fmt.Errorf("component too large to fit in a single Discord message")
			}
			out = append(out, current)
			current = nil
		}
		current = append(current, c)
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out, nil
}

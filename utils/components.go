package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
)

// maxComponentDepth caps recursion through parsed-JSON component trees. The
// 1 MiB body limit on web routes already bounds nesting in practice, but a
// hard cap means hostile or pathological input can't pin a CPU walking it.
// Discord's own component nesting tops out far below this.
const maxComponentDepth = 32

// errComponentTooDeep is returned when a JSON component tree exceeds
// maxComponentDepth during traversal.
var errComponentTooDeep = errors.New("components nested too deeply")

// ResolveEmojis walks a JSON value tree looking for "emoji" keys whose value
// is an object with "name" but no "id", and resolves the name against the
// provided emoji map.
func ResolveEmojis(v any, emojiMap map[string]discord.Emoji) error {
	return resolveEmojis(v, emojiMap, 0)
}

func resolveEmojis(v any, emojiMap map[string]discord.Emoji, depth int) error {
	if depth > maxComponentDepth {
		return errComponentTooDeep
	}
	switch val := v.(type) {
	case map[string]any:
		if emojiObj, ok := val["emoji"].(map[string]any); ok {
			if name, ok := emojiObj["name"].(string); ok {
				_, hasID := emojiObj["id"]
				if !hasID || emojiObj["id"] == nil {
					cleaned := strings.Trim(name, ":")
					if emoji, found := emojiMap[strings.ToLower(cleaned)]; found {
						emojiObj["name"] = emoji.Name
						emojiObj["id"] = emoji.ID.String()
					}
				}
			}
		}
		for _, child := range val {
			if err := resolveEmojis(child, emojiMap, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range val {
			if err := resolveEmojis(item, emojiMap, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderComponentTemplates applies mustache template rendering to all string
// values in the "content" fields of a parsed component JSON tree.
func RenderComponentTemplates(componentsJson string, data MessageTemplateData) (string, error) {
	var parsed any
	if err := json.Unmarshal([]byte(componentsJson), &parsed); err != nil {
		return "", err
	}

	if err := renderTemplatesInTree(parsed, data, 0); err != nil {
		return "", err
	}

	result, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func renderTemplatesInTree(v any, data MessageTemplateData, depth int) error {
	if depth > maxComponentDepth {
		return errComponentTooDeep
	}
	switch val := v.(type) {
	case map[string]any:
		if content, ok := val["content"].(string); ok {
			rendered, err := mustache.RenderRaw(content, true, data)
			if err != nil {
				return err
			}
			val["content"] = rendered
		}
		for _, child := range val {
			if err := renderTemplatesInTree(child, data, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range val {
			if err := renderTemplatesInTree(item, data, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// BuildV2Message renders mustache templates in the component JSON, resolves
// emoji names, and parses the result into layout components ready for sending.
func BuildV2Message(componentsJson string, data MessageTemplateData, emojiMap map[string]discord.Emoji) ([]discord.LayoutComponent, error) {
	rendered, err := RenderComponentTemplates(componentsJson, data)
	if err != nil {
		return nil, err
	}

	var parsed any
	if err := json.Unmarshal([]byte(rendered), &parsed); err != nil {
		return nil, err
	}
	if err := ResolveEmojis(parsed, emojiMap); err != nil {
		return nil, err
	}

	resolvedJSON, err := json.Marshal(parsed)
	if err != nil {
		return nil, err
	}

	return ParseComponents(string(resolvedJSON))
}

// flattenSections pre-processes the JSON component tree. Sections without an
// accessory are invalid (Discord requires one), so they are flattened into
// their child text_display components. This also prevents a panic in disgo's
// SectionComponent.UnmarshalJSON which assumes a non-nil accessory.
func flattenSections(items []any, depth int) ([]any, error) {
	if depth > maxComponentDepth {
		return nil, errComponentTooDeep
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			result = append(result, item)
			continue
		}

		typeNum, _ := obj["type"].(float64)

		// Recurse into containers
		if int(typeNum) == int(discord.ComponentTypeContainer) {
			if children, ok := obj["components"].([]any); ok {
				flattened, err := flattenSections(children, depth+1)
				if err != nil {
					return nil, err
				}
				obj["components"] = flattened
			}
		}

		// Section without a usable accessory → promote children to text_display.
		// Discord requires a non-null accessory, and disgo's SectionComponent
		// unmarshaler panics on nil — so treat both "missing" and "null" as
		// invalid and flatten.
		if int(typeNum) == int(discord.ComponentTypeSection) {
			acc, hasAccessory := obj["accessory"]
			if !hasAccessory || acc == nil {
				if children, ok := obj["components"].([]any); ok {
					result = append(result, children...)
				}
				continue
			}
		}

		result = append(result, obj)
	}
	return result, nil
}

// ParseComponents unmarshals a JSON string into Discord layout components.
// Sections without an accessory are flattened into text displays to satisfy
// both Discord's API requirements and disgo's unmarshaler.
func ParseComponents(jsonStr string) (components []discord.LayoutComponent, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed to parse components: %v", r)
		}
	}()

	var parsed []any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, err
	}

	flattened, err := flattenSections(parsed, 0)
	if err != nil {
		return nil, err
	}
	patched, err := json.Marshal(flattened)
	if err != nil {
		return nil, err
	}

	var raw []discord.UnmarshalComponent
	if err := json.Unmarshal(patched, &raw); err != nil {
		return nil, err
	}

	components = make([]discord.LayoutComponent, 0, len(raw))
	for _, r := range raw {
		lc, ok := r.Component.(discord.LayoutComponent)
		if !ok {
			continue
		}
		components = append(components, lc)
	}
	return components, nil
}

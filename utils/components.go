package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
)

// ResolveEmojis walks a JSON value tree looking for "emoji" keys whose value
// is an object with "name" but no "id", and resolves the name against the
// provided emoji map.
func ResolveEmojis(v any, emojiMap map[string]discord.Emoji) {
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
			ResolveEmojis(child, emojiMap)
		}
	case []any:
		for _, item := range val {
			ResolveEmojis(item, emojiMap)
		}
	}
}

// RenderComponentTemplates applies mustache template rendering to all string
// values in the "content" fields of a parsed component JSON tree.
func RenderComponentTemplates(componentsJson string, data MessageTemplateData) (string, error) {
	var parsed any
	if err := json.Unmarshal([]byte(componentsJson), &parsed); err != nil {
		return "", err
	}

	if err := renderTemplatesInTree(parsed, data); err != nil {
		return "", err
	}

	result, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func renderTemplatesInTree(v any, data MessageTemplateData) error {
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
			if err := renderTemplatesInTree(child, data); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range val {
			if err := renderTemplatesInTree(item, data); err != nil {
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
	ResolveEmojis(parsed, emojiMap)

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
func flattenSections(items []any) []any {
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
				obj["components"] = flattenSections(children)
			}
		}

		// Section without accessory → promote children to text_display
		if int(typeNum) == int(discord.ComponentTypeSection) {
			if _, hasAccessory := obj["accessory"]; !hasAccessory {
				if children, ok := obj["components"].([]any); ok {
					result = append(result, children...)
				}
				continue
			}
		}

		result = append(result, obj)
	}
	return result
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

	patched, err := json.Marshal(flattenSections(parsed))
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

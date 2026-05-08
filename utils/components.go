package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// BuildEmojiMap returns a lowercase emoji-name → Emoji lookup over the guild's
// cached custom emojis. Used by every V2-message sender so emoji name→ID
// resolution stays consistent across join/leave, gatekeep approvals, and the
// dashboard sandbox.
func BuildEmojiMap(client *bot.Client, guildID snowflake.ID) map[string]discord.Emoji {
	emojiMap := make(map[string]discord.Emoji)
	for emoji := range client.Caches.Emojis(guildID) {
		emojiMap[strings.ToLower(emoji.Name)] = emoji
	}
	return emojiMap
}

// maxComponentDepth caps recursion through parsed-JSON component trees. The
// 1 MiB body limit on web routes already bounds nesting in practice, but a
// hard cap means hostile or pathological input can't pin a CPU walking it.
// Discord's own component nesting tops out far below this.
const maxComponentDepth = 32

// errComponentTooDeep is returned when a JSON component tree exceeds
// maxComponentDepth during traversal.
var errComponentTooDeep = errors.New("components nested too deeply")

// errEmptyComponents is returned when a component JSON array is parseable but
// contains zero elements. Discord rejects empty messages, and the save flow
// already guards against this via validateAndCompactV2JSON; this is the same
// guard for paths (sandbox) that bypass the save-flow validator.
var errEmptyComponents = errors.New("components JSON must contain at least one element")

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
		for k, child := range val {
			// The emoji sub-tree was handled inline above; recursing into it
			// would walk the just-mutated object for no benefit. Discord
			// emoji objects are terminal ({name, id, animated}), so skipping
			// is also semantically correct — there are no nested components
			// to find inside one.
			if k == "emoji" {
				continue
			}
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

// errNotTopLevelArray is returned when component JSON parses but isn't a
// top-level array. The sandbox surfaces this to admins; save-flow callers
// have already validated shape via validateAndCompactV2JSON.
var errNotTopLevelArray = errors.New("components JSON must be a top-level array")

// BuildV2Message renders mustache templates in the component JSON, resolves
// emoji names, and parses the result into layout components ready for sending.
func BuildV2Message(componentsJson string, data MessageTemplateData, emojiMap map[string]discord.Emoji) ([]discord.LayoutComponent, error) {
	var parsed any
	if err := json.Unmarshal([]byte(componentsJson), &parsed); err != nil {
		return nil, err
	}
	if err := renderTemplatesInTree(parsed, data, 0); err != nil {
		return nil, err
	}
	if err := ResolveEmojis(parsed, emojiMap); err != nil {
		return nil, err
	}
	arr, ok := parsed.([]any)
	if !ok {
		return nil, errNotTopLevelArray
	}
	return parseComponentsFromAny(arr)
}

// ValidateV2Components runs the parsed component array through the same
// disgo-unmarshal pipeline that BuildV2Message{,NoTemplate} use at send time,
// without emoji resolution. Save-time callers use this so payloads that would
// only fail when serialized to Discord (e.g. a text_display whose content is
// a number, or any field that disgo's typed unmarshaler rejects) surface at
// save instead of at publish.
//
// Empty arrays are accepted: drafts use them, and the runtime senders refuse
// to send an empty message on their own.
//
// Note: parseComponentsFromAny may mutate `arr` (flattenSections rewrites
// `obj["components"]` for accessory-less sections). Callers that reuse `arr`
// after validation should pass a copy.
func ValidateV2Components(arr []any) error {
	if len(arr) == 0 {
		return nil
	}
	_, err := parseComponentsFromAny(arr)
	return err
}

// BuildV2MessageNoTemplate is BuildV2Message minus mustache rendering — the
// sandbox sends the admin's literal component JSON, not a template, so it
// shouldn't pay the parse-render-marshal round-trip.
func BuildV2MessageNoTemplate(componentsJson string, emojiMap map[string]discord.Emoji) ([]discord.LayoutComponent, error) {
	var parsed any
	if err := json.Unmarshal([]byte(componentsJson), &parsed); err != nil {
		return nil, err
	}
	if err := ResolveEmojis(parsed, emojiMap); err != nil {
		return nil, err
	}
	arr, ok := parsed.([]any)
	if !ok {
		return nil, errNotTopLevelArray
	}
	return parseComponentsFromAny(arr)
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
func ParseComponents(jsonStr string) ([]discord.LayoutComponent, error) {
	var parsed []any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, err
	}
	return parseComponentsFromAny(parsed)
}

// parseComponentsFromAny is the shared core for ParseComponents and the
// BuildV2Message variants: it operates on an already-decoded JSON array,
// avoiding a redundant unmarshal cycle when the caller has parsed the JSON
// for prior in-place mutation (template rendering, emoji resolution).
func parseComponentsFromAny(parsed []any) (components []discord.LayoutComponent, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed to parse components: %v", r)
		}
	}()

	if len(parsed) == 0 {
		return nil, errEmptyComponents
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

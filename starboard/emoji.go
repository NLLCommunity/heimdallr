package starboard

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/disgoorg/snowflake/v2"
)

var customEmoji = regexp.MustCompile(`^([A-Za-z0-9_]{2,32}):([0-9]+)$`)

// Unicode Emoji 17.0 fully-qualified sequences, derived from:
// https://www.unicode.org/Public/17.0.0/emoji/emoji-test.txt
// Copyright © 2025 Unicode, Inc.; see UNICODE-LICENSE.txt.
//
//go:embed unicode_emoji.txt
var unicodeEmojiData string

var unicodeEmojis = func() map[string]string {
	result := make(map[string]string)
	for _, emoji := range strings.Fields(unicodeEmojiData) {
		result[normalizeEmoji(emoji)] = emoji
	}
	return result
}()

func normalizeEmoji(raw string) string {
	return strings.NewReplacer("\ufe0f", "", "\ufe0e", "").Replace(raw)
}

// ParseEmoji stores custom emojis in Discord's reaction form and normalizes
// presentation selectors so ❌ cannot be selected under an alternate spelling.
// Discord validation in ValidateBoard rejects unavailable custom reactions.
func ParseEmoji(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<") {
		if !strings.HasSuffix(raw, ">") {
			return "", fmt.Errorf("invalid custom emoji")
		}
		raw = strings.TrimSuffix(strings.TrimPrefix(raw, "<"), ">")
		if strings.HasPrefix(raw, "a:") {
			raw = strings.TrimPrefix(raw, "a:")
		} else if strings.HasPrefix(raw, ":") {
			raw = strings.TrimPrefix(raw, ":")
		} else {
			return "", fmt.Errorf("invalid custom emoji")
		}
	}
	if parts := customEmoji.FindStringSubmatch(raw); parts != nil {
		id, err := snowflake.Parse(parts[2])
		if err != nil || id == 0 {
			return "", fmt.Errorf("invalid custom emoji ID")
		}
		return parts[1] + ":" + id.String(), nil
	}
	raw = normalizeEmoji(raw)
	if raw == "❌" {
		return "", fmt.Errorf("❌ is reserved for downvotes")
	}
	canonical, ok := unicodeEmojis[raw]
	if !ok {
		return "", fmt.Errorf("choose one Unicode emoji or a server custom emoji")
	}
	return canonical, nil
}

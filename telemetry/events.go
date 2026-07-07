package telemetry

import (
	"context"
	"regexp"
	"strings"
	"sync"
)

type Event struct {
	Name       string
	DistinctID string
	GuildID    string
	Properties map[string]any
}

type Events interface {
	Capture(context.Context, Event)
}

type noopEvents struct{}

type sanitizingEvents struct {
	next Events
}

var (
	defaultEventsMu         sync.RWMutex
	defaultEvents           Events = sanitizingEvents{next: noopEvents{}}
	defaultEventsGeneration uint64
)

func NewNoopEvents() Events {
	return noopEvents{}
}

func (noopEvents) Capture(context.Context, Event) {}

func (s sanitizingEvents) Capture(ctx context.Context, event Event) {
	s.next.Capture(ctx, sanitizeEvent(event))
}

func SetDefaultEvents(events Events) func() {
	if events == nil {
		events = noopEvents{}
	}

	defaultEventsMu.Lock()
	previous := defaultEvents
	previousGeneration := defaultEventsGeneration
	defaultEventsGeneration++
	generation := defaultEventsGeneration
	defaultEvents = sanitizingEvents{next: events}
	defaultEventsMu.Unlock()

	restored := false

	return func() {
		defaultEventsMu.Lock()
		defer defaultEventsMu.Unlock()

		if restored {
			return
		}
		restored = true
		if defaultEventsGeneration != generation {
			return
		}

		defaultEvents = previous
		defaultEventsGeneration = previousGeneration
	}
}

func Capture(ctx context.Context, event Event) {
	defaultEventsMu.RLock()
	events := defaultEvents
	defaultEventsMu.RUnlock()

	events.Capture(ctx, event)
}

func sanitizeEvent(event Event) Event {
	event.Properties = sanitizePropertiesForEvent(event.Name, event.Properties)
	return event
}

var blockedPropertyKeys = map[string]struct{}{
	"access_token":     {},
	"after_content":    {},
	"authorization":    {},
	"channel_name":     {},
	"client_ip":        {},
	"before_content":   {},
	"components_json":  {},
	"content":          {},
	"cookie":           {},
	"current_message":  {},
	"details":          {},
	"discord_name":     {},
	"guild_name":       {},
	"ip":               {},
	"message":          {},
	"modal_text":       {},
	"oauth_state":      {},
	"post_name":        {},
	"previous_message": {},
	"query":            {},
	"query_string":     {},
	"raw_query":        {},
	"reason":           {},
	"refresh_token":    {},
	"request_body":     {},
	"role_name":        {},
	"session_token":    {},
	"template":         {},
	"token":            {},
	"x_forwarded_for":  {},
	"x_real_ip":        {},
}

var allowedEventPropertyKeys = map[string]map[string]struct{}{
	"dashboard_signed_in": {
		"oauth_scopes_ok":   {},
		"return_to_present": {},
	},
	"guild_selected": {
		"access_level": {},
		"guild_id":     {},
	},
	"settings_saved": {
		"guild_id": {},
		"section":  {},
		"source":   {},
	},
	"post_created": {
		"component_count":       {},
		"guild_id":              {},
		"has_channel":           {},
		"planned_message_count": {},
		"post_id":               {},
	},
	"post_saved": {
		"component_count":       {},
		"guild_id":              {},
		"has_channel":           {},
		"planned_message_count": {},
		"post_id":               {},
	},
	"post_published": {
		"created_count":         {},
		"delete_failure_count":  {},
		"deleted_count":         {},
		"guild_id":              {},
		"planned_message_count": {},
		"post_id":               {},
		"recreated_all":         {},
	},
	"post_unpublished": {
		"guild_id":               {},
		"post_id":                {},
		"previous_message_count": {},
	},
	"sandbox_message_sent": {
		"component_count":       {},
		"guild_id":              {},
		"planned_message_count": {},
	},
	"command_used": {
		"channel_id":       {},
		"command_name":     {},
		"guild_id":         {},
		"interaction_type": {},
		"namespace":        {},
	},
}

var allowedStringPropertyKeys = map[string]struct{}{
	"access_level":     {},
	"command_name":     {},
	"interaction_type": {},
	"namespace":        {},
	"section":          {},
	"source":           {},
}

var safeTelemetryTokenPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,127}$`)

func sanitizeProperties(properties map[string]any) map[string]any {
	if len(properties) == 0 {
		return nil
	}

	safe := make(map[string]any, len(properties))
	for key, value := range properties {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		if isBlockedPropertyKey(normalized) {
			continue
		}

		switch typed := value.(type) {
		case bool:
			safe[normalized] = typed
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			if strings.HasSuffix(normalized, "_count") {
				safe[normalized] = value
			}
		case string:
			if sanitized, ok := sanitizeStringProperty(normalized, typed); ok {
				safe[normalized] = sanitized
			}
		}
	}

	if len(safe) == 0 {
		return nil
	}

	return safe
}

func sanitizePropertiesForEvent(eventName string, properties map[string]any) map[string]any {
	safe := sanitizeProperties(properties)
	if len(safe) == 0 {
		return nil
	}

	allowedKeys, ok := allowedEventPropertyKeys[strings.TrimSpace(eventName)]
	if !ok {
		return nil
	}

	filtered := make(map[string]any, len(safe))
	for key, value := range safe {
		if _, allowed := allowedKeys[key]; allowed {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	return filtered
}

func sanitizeStringProperty(key, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if len(value) > 128 {
		return "", false
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return "", false
	}
	if !safeTelemetryTokenPattern.MatchString(value) {
		return "", false
	}

	if strings.HasSuffix(key, "_id") {
		return value, true
	}
	if _, ok := allowedStringPropertyKeys[key]; ok {
		return value, true
	}

	return "", false
}

func isBlockedPropertyKey(key string) bool {
	if _, blocked := blockedPropertyKeys[key]; blocked {
		return true
	}

	if strings.Contains(key, "token") ||
		strings.Contains(key, "cookie") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "query") ||
		strings.Contains(key, "ip") {
		return true
	}

	if strings.HasSuffix(key, "_name") {
		_, allowed := allowedStringPropertyKeys[key]
		return !allowed
	}

	return false
}

func eventAllowsProperty(eventName, propertyKey string) bool {
	allowedKeys, ok := allowedEventPropertyKeys[strings.TrimSpace(eventName)]
	if !ok {
		return false
	}
	_, allowed := allowedKeys[strings.ToLower(strings.TrimSpace(propertyKey))]
	return allowed
}

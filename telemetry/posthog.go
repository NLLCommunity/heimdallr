package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/posthog/posthog-go"
)

var discordSnowflakeIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

type posthogEvents struct {
	client         posthog.EnqueueClient
	logger         *slog.Logger
	groupAnalytics bool
}

type posthogShutdowner struct {
	client interface {
		Close() error
	}
}

func startPostHog(cfg PostHogConfig, logger *slog.Logger) (Events, shutdowner, error) {
	client, err := posthog.NewWithConfig(cfg.APIKey, posthog.Config{
		Endpoint:        cfg.Endpoint,
		Interval:        time.Duration(cfg.FlushIntervalSeconds) * time.Second,
		BatchSize:       cfg.FlushAt,
		ShutdownTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("posthog client: %w", err)
	}

	return &posthogEvents{
		client:         client,
		logger:         logger,
		groupAnalytics: cfg.GroupAnalyticsEnabled,
	}, posthogShutdowner{client: client}, nil
}

func (e *posthogEvents) Capture(ctx context.Context, event Event) {
	if e == nil || e.client == nil {
		return
	}

	eventName, ok := sanitizePostHogEventName(event.Name)
	if !ok {
		return
	}

	distinctID, ok := sanitizePostHogDistinctID(event.DistinctID)
	if !ok {
		return
	}

	properties := posthog.NewProperties()
	for key, value := range sanitizePropertiesForEvent(eventName, event.Properties) {
		if key == "guild_id" {
			guildID, ok := value.(string)
			if !ok {
				continue
			}
			if guildID, ok = sanitizePostHogGuildID(guildID); !ok {
				continue
			}
			properties = properties.Set(key, guildID)
			continue
		}
		properties = properties.Set(key, value)
	}
	if guildID, ok := sanitizePostHogGuildID(event.GuildID); ok && eventAllowsProperty(eventName, "guild_id") {
		properties = properties.Set("guild_id", guildID)
	}

	capture := posthog.Capture{
		DistinctId: distinctID,
		Event:      eventName,
		Properties: properties,
	}
	if e.groupAnalytics {
		if guildID, ok := sanitizePostHogGuildID(event.GuildID); ok && eventAllowsProperty(eventName, "guild_id") {
			capture.Groups = posthog.NewGroups().Set("guild", guildID)
		}
	}

	if err := e.client.Enqueue(capture); err != nil && e.logger != nil {
		e.logger.WarnContext(ctx, "posthog enqueue failed", "error", err, "event", eventName)
	}
}

func (s posthogShutdowner) Shutdown(context.Context) error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func sanitizePostHogEventName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	_, ok := allowedEventPropertyKeys[name]
	return name, ok
}

func sanitizePostHogDistinctID(distinctID string) (string, bool) {
	return sanitizePostHogGuildID(distinctID)
}

func sanitizePostHogGuildID(guildID string) (string, bool) {
	guildID = strings.TrimSpace(guildID)
	if !discordSnowflakeIDPattern.MatchString(guildID) {
		return "", false
	}
	return guildID, true
}

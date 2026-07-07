package telemetry

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingEvents struct {
	events []Event
}

type fakePostHogClient struct {
	messages []posthog.Message
	err      error
}

func (r *recordingEvents) Capture(_ context.Context, event Event) {
	r.events = append(r.events, event)
}

func (f *fakePostHogClient) Enqueue(message posthog.Message) error {
	f.messages = append(f.messages, message)
	return f.err
}

func TestNoopEventsDoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		NewNoopEvents().Capture(context.Background(), Event{
			Name:       "command_used",
			DistinctID: "123",
		})
	})
}

func TestDefaultEventsCanBeReplacedAndRestored(t *testing.T) {
	t.Cleanup(func() {
		SetDefaultEvents(NewNoopEvents())
	})

	recorder := &recordingEvents{}
	restore := SetDefaultEvents(recorder)

	Capture(context.Background(), Event{
		Name:       "command_used",
		DistinctID: "123",
		Properties: map[string]any{
			"namespace": "ping",
		},
	})

	require.Len(t, recorder.events, 1)
	assert.Equal(t, "command_used", recorder.events[0].Name)
	assert.Equal(t, "123", recorder.events[0].DistinctID)

	restore()
	require.NotPanics(t, func() {
		Capture(context.Background(), Event{Name: "after_restore", DistinctID: "123"})
	})
}

func TestDefaultEventsRestoreIsOneShotAndOrdered(t *testing.T) {
	t.Cleanup(func() {
		SetDefaultEvents(NewNoopEvents())
	})

	first := &recordingEvents{}
	restoreFirst := SetDefaultEvents(first)

	second := &recordingEvents{}
	restoreSecond := SetDefaultEvents(second)

	restoreFirst()
	Capture(context.Background(), Event{Name: "still_second"})
	require.Len(t, second.events, 1)
	require.Empty(t, first.events)

	restoreSecond()
	Capture(context.Background(), Event{Name: "restored_to_first"})
	require.Len(t, first.events, 1)
	require.Len(t, second.events, 1)
	assert.Equal(t, "restored_to_first", first.events[0].Name)

	restoreSecond()
	Capture(context.Background(), Event{Name: "still_first"})
	require.Len(t, first.events, 2)
	require.Len(t, second.events, 1)

	restoreFirst()
	Capture(context.Background(), Event{Name: "still_first_after_double_restore"})
	require.Len(t, first.events, 3)
	require.Len(t, second.events, 1)
}

func TestSanitizePropertiesAllowsOnlySafeTelemetryShapes(t *testing.T) {
	got := sanitizeProperties(map[string]any{
		"guild_id":          "1",
		"post_id":           "post:2",
		"component_count":   2,
		"return_to_present": true,
		"access_level":      "admin",
		"command_name":      "warnings.list",
		"interaction_type":  "slash_command",
		"namespace":         "moderation",
		"section":           "warnings:active",
		"source":            "discord.ui",
		"guild_name":        "NLL",
		"channel_name":      "general",
		"role_name":         "moderator",
		"discord_name":      "codex",
		"query_string":      "page=1",
		"client_ip":         "127.0.0.1",
		"current_message":   "secret",
		"components_json":   "{}",
		"authorization":     "Bearer secret",
		"details":           "raw audit payload",
		"notes":             "arbitrary free-form string",
	})

	assert.Equal(t, map[string]any{
		"guild_id":          "1",
		"post_id":           "post:2",
		"component_count":   2,
		"return_to_present": true,
		"access_level":      "admin",
		"command_name":      "warnings.list",
		"interaction_type":  "slash_command",
		"namespace":         "moderation",
		"section":           "warnings:active",
		"source":            "discord.ui",
	}, got)
}

func TestSanitizePropertiesDropsFreeFormStringsRelabeledAsApprovedKeys(t *testing.T) {
	got := sanitizeProperties(map[string]any{
		"access_level":     "admin team",
		"command_name":     "warnings list",
		"interaction_type": "button click",
		"namespace":        "mod tools",
		"section":          "warnings active",
		"source":           "discord ui",
		"guild_id":         "guild-123",
		"post_id":          "post:456",
	})

	assert.Equal(t, map[string]any{
		"guild_id": "guild-123",
		"post_id":  "post:456",
	}, got)
}

func TestSanitizeEventKeepsOnlyPropertiesAllowlistedForEvent(t *testing.T) {
	got := sanitizeEvent(Event{
		Name: "dashboard_signed_in",
		Properties: map[string]any{
			"return_to_present": true,
			"oauth_scopes_ok":   true,
			"guild_id":          "123456789012345678",
			"component_count":   2,
			"namespace":         "discord",
		},
	})

	assert.Equal(t, map[string]any{
		"return_to_present": true,
		"oauth_scopes_ok":   true,
	}, got.Properties)
}

func TestPostHogEventsCaptureSafeProperties(t *testing.T) {
	client := &fakePostHogClient{}
	events := &posthogEvents{
		client:         client,
		logger:         slog.Default(),
		groupAnalytics: true,
	}

	events.Capture(context.Background(), Event{
		Name:       "post_created",
		DistinctID: "123456789012345678",
		GuildID:    "987654321098765432",
		Properties: map[string]any{
			"component_count": 2,
			"content":         "secret",
		},
	})

	require.Len(t, client.messages, 1)
	capture, ok := client.messages[0].(posthog.Capture)
	require.True(t, ok)
	assert.Equal(t, "post_created", capture.Event)
	assert.Equal(t, "123456789012345678", capture.DistinctId)
	assert.Equal(t, 2, capture.Properties["component_count"])
	assert.Equal(t, "987654321098765432", capture.Properties["guild_id"])
	assert.NotContains(t, capture.Properties, "content")
	assert.Equal(t, posthog.Groups{"guild": "987654321098765432"}, capture.Groups)
}

func TestPostHogEventsCaptureDropsSafeLookingPropertiesOutsideEventAllowlist(t *testing.T) {
	client := &fakePostHogClient{}
	events := &posthogEvents{
		client: client,
		logger: slog.Default(),
	}

	events.Capture(context.Background(), Event{
		Name:       "sandbox_message_sent",
		DistinctID: "123456789012345678",
		GuildID:    "987654321098765432",
		Properties: map[string]any{
			"guild_id":              "987654321098765432",
			"component_count":       2,
			"planned_message_count": 1,
			"post_id":               "456",
			"delete_failure_count":  9,
			"channel_id":            "555555555555555555",
		},
	})

	require.Len(t, client.messages, 1)
	capture, ok := client.messages[0].(posthog.Capture)
	require.True(t, ok)
	assert.Equal(t, "sandbox_message_sent", capture.Event)
	assert.Equal(t, "987654321098765432", capture.Properties["guild_id"])
	assert.Equal(t, 2, capture.Properties["component_count"])
	assert.Equal(t, 1, capture.Properties["planned_message_count"])
	assert.NotContains(t, capture.Properties, "post_id")
	assert.NotContains(t, capture.Properties, "delete_failure_count")
	assert.NotContains(t, capture.Properties, "channel_id")
}

func TestPostHogEventsCaptureDropsUnsafeEventMetadata(t *testing.T) {
	testCases := []Event{
		{
			Name:       "discord display name",
			DistinctID: "123456789012345678",
		},
		{
			Name:       "post_created",
			DistinctID: "alice",
		},
		{
			Name:       "post_created",
			DistinctID: "general",
		},
		{
			Name:       "post_created",
			DistinctID: "moderators",
		},
		{
			Name:       "post_created",
			DistinctID: "user-1",
		},
		{
			Name:       "post_created",
			DistinctID: "Bearersecret",
		},
		{
			Name:       "post_created",
			DistinctID: strings.Repeat("1", 33),
		},
	}

	for _, testCase := range testCases {
		client := &fakePostHogClient{}
		events := &posthogEvents{
			client: client,
			logger: slog.Default(),
		}

		events.Capture(context.Background(), testCase)

		assert.Empty(t, client.messages, "event %#v should not enqueue", testCase)
	}
}

func TestPostHogEventsCaptureDropsUnsafeGuildIDs(t *testing.T) {
	client := &fakePostHogClient{}
	events := &posthogEvents{
		client:         client,
		logger:         slog.Default(),
		groupAnalytics: true,
	}

	events.Capture(context.Background(), Event{
		Name:       "post_created",
		DistinctID: "123456789012345678",
		GuildID:    "general",
		Properties: map[string]any{
			"guild_id":        "moderators",
			"component_count": 2,
		},
	})

	require.Len(t, client.messages, 1)
	capture, ok := client.messages[0].(posthog.Capture)
	require.True(t, ok)
	assert.Equal(t, "post_created", capture.Event)
	assert.Equal(t, "123456789012345678", capture.DistinctId)
	assert.Equal(t, 2, capture.Properties["component_count"])
	assert.NotContains(t, capture.Properties, "guild_id")
	assert.Nil(t, capture.Groups)
}

func TestPostHogEventsCaptureAllowsPlannedEventNamesAndSafeDistinctIDs(t *testing.T) {
	eventNames := []string{
		"dashboard_signed_in",
		"guild_selected",
		"settings_saved",
		"post_created",
		"post_saved",
		"post_published",
		"post_unpublished",
		"sandbox_message_sent",
		"command_used",
	}

	client := &fakePostHogClient{}
	events := &posthogEvents{
		client: client,
		logger: slog.Default(),
	}

	for _, eventName := range eventNames {
		events.Capture(context.Background(), Event{
			Name:       eventName,
			DistinctID: "123456789012345678",
		})
	}

	require.Len(t, client.messages, len(eventNames))
	for index, eventName := range eventNames {
		capture, ok := client.messages[index].(posthog.Capture)
		require.True(t, ok)
		assert.Equal(t, eventName, capture.Event)
		assert.Equal(t, "123456789012345678", capture.DistinctId)
	}
}

func TestPostHogEventsCaptureRejectsRemovedEventNames(t *testing.T) {
	for _, eventName := range []string{"post_updated", "post_deleted", "post_sync_completed"} {
		client := &fakePostHogClient{}
		events := &posthogEvents{
			client: client,
			logger: slog.Default(),
		}

		events.Capture(context.Background(), Event{
			Name:       eventName,
			DistinctID: "123456789012345678",
		})

		assert.Empty(t, client.messages, "removed event %q should not enqueue", eventName)
	}
}

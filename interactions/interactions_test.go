package interactions

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/NLLCommunity/heimdallr/telemetry"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestNewDMError(t *testing.T) {
	innerErr := errors.New("inner error")

	tests := []struct {
		name             string
		dmChannelCreated bool
		messageSent      bool
		innerError       error
		expectedError    string
	}{
		{
			name:             "failed to create DM channel",
			dmChannelCreated: false,
			messageSent:      false,
			innerError:       innerErr,
			expectedError:    "failed to create DM channel",
		},
		{
			name:             "failed to send message",
			dmChannelCreated: true,
			messageSent:      false,
			innerError:       innerErr,
			expectedError:    "failed to send message",
		},
		{
			name:             "unknown error",
			dmChannelCreated: true,
			messageSent:      true,
			innerError:       innerErr,
			expectedError:    "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				err := NewDMError(tt.dmChannelCreated, tt.messageSent, tt.innerError)
				assert.Equal(t, tt.expectedError, err.Error())
				assert.Equal(t, tt.innerError, errors.Unwrap(err))
			},
		)
	}
}

func TestEphemeralMessageContent(t *testing.T) {
	content := "Test message"
	builder := EphemeralMessageContent(content)

	message := builder

	assert.Equal(t, content, message.Content)
	assert.True(t, message.Flags.Has(discord.MessageFlagEphemeral))
	assert.NotNil(t, message.AllowedMentions)
	assert.Empty(t, message.AllowedMentions.Parse)
	assert.Empty(t, message.AllowedMentions.Users)
	assert.Empty(t, message.AllowedMentions.Roles)
}

func TestEphemeralMessageContentf(t *testing.T) {
	template := "User %s has %d points"
	username := "testuser"
	points := 42

	builder := EphemeralMessageContentf(template, username, points)
	message := builder

	expected := "User testuser has 42 points"
	assert.Equal(t, expected, message.Content)
	assert.True(t, message.Flags.Has(discord.MessageFlagEphemeral))
}

// MockBot is a mock implementation of bot.Client for testing
type MockBot struct {
	mock.Mock
}

func (m *MockBot) Rest() *MockRest {
	args := m.Called()
	return args.Get(0).(*MockRest)
}

type MockRest struct {
	mock.Mock
}

func (m *MockRest) CreateDMChannel(userID snowflake.ID) (discord.DMChannel, error) {
	args := m.Called(userID)
	return args.Get(0).(discord.DMChannel), args.Error(1)
}

func (m *MockRest) CreateMessage(channelID snowflake.ID, messageCreate discord.MessageCreate) (
	*discord.Message, error,
) {
	args := m.Called(channelID, messageCreate)
	return args.Get(0).(*discord.Message), args.Error(1)
}

func TestApplicationCommandRegisterFunc(t *testing.T) {
	// Test that ApplicationCommandRegisterFunc implements AppCommandRegisterer.
	var _ AppCommandRegisterer = ApplicationCommandRegisterFunc(nil)

	// Test the Register method.
	expectedCommands := []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:        "test",
			Description: "test command",
		},
	}

	registerFunc := ApplicationCommandRegisterFunc(
		func(r *handler.Mux) []discord.ApplicationCommandCreate {
			return expectedCommands
		},
	)

	result := registerFunc.Register(nil)
	assert.Equal(t, expectedCommands, result)
}

func TestErrEventNoGuildID(t *testing.T) {
	assert.Equal(t, "no guild id found in event", ErrEventNoGuildID.Error())
}

type recordingTelemetryEvents struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *recordingTelemetryEvents) Capture(_ context.Context, event telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
}

func (r *recordingTelemetryEvents) snapshot() []telemetry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]telemetry.Event, len(r.events))
	copy(events, r.events)
	return events
}

func TestInteractionTelemetryEventPropertiesAreSafe(t *testing.T) {
	props := commandUsedProperties("admin", "slash command", "warnings list", "123", "456")

	assert.Equal(t, map[string]any{
		"namespace":        "admin",
		"interaction_type": "application_command",
		"command_name":     "warnings_list",
		"guild_id":         "123",
		"channel_id":       "456",
	}, props)
}

func TestRecoverGoCapturesSafeTelemetryEvent(t *testing.T) {
	recorder := &recordingTelemetryEvents{}
	restoreEvents := telemetry.SetDefaultEvents(recorder)
	t.Cleanup(restoreEvents)

	done := make(chan struct{})
	var handlerCtx context.Context
	traceProvider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})
	parentCtx, parentSpan := traceProvider.Tracer("interaction-test").Start(context.Background(), "parent")
	defer parentSpan.End()

	wrapped := RecoverGo(func(event *handler.InteractionEvent) error {
		handlerCtx = event.Ctx
		close(done)
		return nil
	})

	event := newInteractionEvent(t)
	event.Ctx = parentCtx
	err := wrapped(event)
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interaction handler")
	}

	require.True(t, trace.SpanContextFromContext(handlerCtx).IsValid())
	assert.Equal(t, trace.SpanContextFromContext(parentCtx).TraceID(), trace.SpanContextFromContext(handlerCtx).TraceID())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, telemetry.Event{
		Name:       "command_used",
		DistinctID: "555555555555555555",
		GuildID:    "333333333333333333",
		Properties: map[string]any{
			"namespace":        "discord",
			"interaction_type": "application_command",
			"command_name":     "warnings",
			"guild_id":         "333333333333333333",
			"channel_id":       "444444444444444444",
		},
	}, events[0])
}

func TestRecoverGoPanicLogUsesSafeFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	done := make(chan struct{})
	wrapped := RecoverGo(func(event *handler.InteractionEvent) error {
		defer close(done)
		panic("panic secret details")
	})

	event := newInteractionEvent(t)
	event.GenericEvent = events.NewGenericEvent(&bot.Client{Logger: logger}, 0, 0)

	err := wrapped(event)
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for panic recovery")
	}

	require.Eventually(t, func() bool {
		return buf.Len() > 0
	}, time.Second, 10*time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, "recovered from panic in interaction handler")
	assert.Contains(t, output, "panic_recovered=true")
	assert.Contains(t, output, "outcome=panic")
	assert.Contains(t, output, "interaction_type=application_command")
	assert.Contains(t, output, "command_name=warnings")
	assert.Contains(t, output, "guild_id=333333333333333333")
	assert.Contains(t, output, "channel_id=444444444444444444")
	assert.Contains(t, output, "user_id=555555555555555555")
	assert.NotContains(t, output, "panic secret details")
	assert.NotContains(t, output, "goroutine")
}

func TestRecoverGoErrorLogUsesSafeFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	done := make(chan struct{})
	wrapped := RecoverGo(func(event *handler.InteractionEvent) error {
		defer close(done)
		return errors.New("error secret details")
	})

	event := newInteractionEvent(t)
	event.GenericEvent = events.NewGenericEvent(&bot.Client{Logger: logger}, 0, 0)

	err := wrapped(event)
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interaction error log")
	}

	require.Eventually(t, func() bool {
		return buf.Len() > 0
	}, time.Second, 10*time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, "failed to handle interaction")
	assert.Contains(t, output, "error=true")
	assert.Contains(t, output, "outcome=error")
	assert.Contains(t, output, "interaction_type=application_command")
	assert.Contains(t, output, "command_name=warnings")
	assert.Contains(t, output, "guild_id=333333333333333333")
	assert.Contains(t, output, "channel_id=444444444444444444")
	assert.Contains(t, output, "user_id=555555555555555555")
	assert.NotContains(t, output, "error secret details")
}

func newInteractionEvent(t *testing.T) *handler.InteractionEvent {
	t.Helper()

	interaction, err := discord.UnmarshalInteraction([]byte(`{
		"id":"111111111111111111",
		"type":2,
		"application_id":"222222222222222222",
		"token":"interaction-token",
		"version":1,
		"guild_id":"333333333333333333",
		"channel":{"id":"444444444444444444","type":0,"name":"general","permissions":"0"},
		"user":{"id":"555555555555555555","username":"tester","discriminator":"0"},
		"data":{"id":"666666666666666666","name":"warnings","type":1}
	}`))
	require.NoError(t, err)

	return &handler.InteractionEvent{
		InteractionCreate: &events.InteractionCreate{
			Interaction: interaction,
		},
	}
}

package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactingHandlerRedactsSensitiveAttributes(t *testing.T) {
	var buf bytes.Buffer
	handler := newRedactingHandler(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	logger := slog.New(handler)

	logger.InfoContext(context.Background(), "message checked",
		"guild_id", "123",
		"current_message", "secret content",
		"authorization", "Bearer secret",
	)

	got := buf.String()
	assert.Contains(t, got, "guild_id=123")
	assert.Contains(t, got, "current_message=[REDACTED]")
	assert.Contains(t, got, "authorization=[REDACTED]")
	assert.NotContains(t, got, "secret content")
	assert.NotContains(t, got, "Bearer secret")
}

func TestRedactingHandlerRedactsSensitiveGroupedAttributes(t *testing.T) {
	var buf bytes.Buffer
	handler := newRedactingHandler(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	logger := slog.New(handler)

	logger.InfoContext(context.Background(), "grouped message",
		slog.Group("request",
			slog.String("guild_id", "123"),
			slog.String("authorization", "Bearer secret"),
			slog.Group("payload",
				slog.String("current_message", "secret content"),
			),
		),
	)

	got := buf.String()
	assert.Contains(t, got, "request.guild_id=123")
	assert.Contains(t, got, "request.authorization=[REDACTED]")
	assert.Contains(t, got, "request.payload.current_message=[REDACTED]")
	assert.NotContains(t, got, "secret content")
	assert.NotContains(t, got, "Bearer secret")
}

func TestRedactingHandlerDefaultModePreservesOperationalErrorKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(buildTextHandler(&buf, slog.LevelDebug))

	logger.ErrorContext(context.Background(), "runtime failure",
		"err", "boom detail",
		"error", "raw error prose",
		"authorization", "Bearer secret",
		slog.Group("payload",
			slog.String("current_message", "secret content"),
		),
	)

	got := buf.String()
	assert.Contains(t, got, "err=\"boom detail\"")
	assert.Contains(t, got, "error=\"raw error prose\"")
	assert.Contains(t, got, "authorization=[REDACTED]")
	assert.Contains(t, got, "payload.current_message=[REDACTED]")
	assert.NotContains(t, got, "Bearer secret")
	assert.NotContains(t, got, "secret content")
}

func TestOTelRedactingHandlerRedactsOperationalKeys(t *testing.T) {
	var buf bytes.Buffer
	handler := newOTelRedactingHandler(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	logger := slog.New(handler)

	logger.ErrorContext(context.Background(), "runtime failure",
		"err", "boom detail",
		"error", "raw error prose",
		"panic", "panic payload",
		slog.Group("telemetry",
			slog.String("stack", "frame1\nframe2"),
			slog.String("error_count", "2"),
			slog.Bool("panic_recovered", true),
			slog.String("outcome", "retryable"),
			slog.String("guild_id", "123"),
		),
	)

	got := buf.String()
	assert.Contains(t, got, "err=[REDACTED]")
	assert.Contains(t, got, "error=[REDACTED]")
	assert.Contains(t, got, "panic=[REDACTED]")
	assert.Contains(t, got, "telemetry.stack=[REDACTED]")
	assert.Contains(t, got, "telemetry.error_count=2")
	assert.Contains(t, got, "telemetry.panic_recovered=true")
	assert.Contains(t, got, "telemetry.outcome=retryable")
	assert.Contains(t, got, "telemetry.guild_id=123")
	assert.NotContains(t, got, "boom detail")
	assert.NotContains(t, got, "raw error prose")
	assert.NotContains(t, got, "panic payload")
	assert.NotContains(t, got, "frame1")
	assert.NotContains(t, got, "frame2")
}

func TestFanoutHandlerWritesToEveryEnabledHandler(t *testing.T) {
	var first bytes.Buffer
	var second bytes.Buffer
	handler := newFanoutHandler(
		slog.NewTextHandler(&first, &slog.HandlerOptions{Level: slog.LevelInfo}),
		slog.NewTextHandler(&second, &slog.HandlerOptions{Level: slog.LevelInfo}),
	)
	logger := slog.New(handler)

	logger.Info("fanout works", "guild_id", "123")

	require.Contains(t, first.String(), "fanout works")
	require.Contains(t, second.String(), "fanout works")
}

func TestBuildTextHandlerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(buildTextHandler(&buf, slog.LevelWarn))

	logger.Info("hidden")
	logger.Warn("visible")

	assert.NotContains(t, strings.ToLower(buf.String()), "hidden")
	assert.Contains(t, strings.ToLower(buf.String()), "visible")
}

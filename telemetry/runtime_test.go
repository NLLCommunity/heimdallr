package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type shutdownFunc func(context.Context) error

func (f shutdownFunc) Shutdown(ctx context.Context) error {
	return f(ctx)
}

func TestStartDisabledProvidersReturnsUsableRuntime(t *testing.T) {
	var buf bytes.Buffer
	runtime, err := Start(context.Background(), Config{}, buildTextHandler(&buf, slog.LevelInfo))
	require.NoError(t, err)
	require.NotNil(t, runtime)

	runtime.Logger().Info("local log")
	runtime.Events().Capture(context.Background(), Event{Name: "command_used", DistinctID: "123"})

	assert.Contains(t, buf.String(), "local log")
	require.NoError(t, runtime.Shutdown(context.Background()))
}

func TestRuntimeShutdownReturnsJoinedErrors(t *testing.T) {
	runtime := &Runtime{
		logger: slog.Default(),
		events: NewNoopEvents(),
		shutdowners: []shutdowner{
			shutdownFunc(func(context.Context) error { return errors.New("first") }),
			shutdownFunc(func(context.Context) error { return errors.New("second") }),
		},
	}

	err := runtime.Shutdown(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "first")
	assert.ErrorContains(t, err, "second")
}

func TestStartReturnsValidationError(t *testing.T) {
	_, err := Start(context.Background(), Config{
		PostHog: PostHogConfig{Enabled: true},
	}, slog.Default().Handler())

	require.ErrorContains(t, err, "telemetry.posthog.api_key is required")
}

func TestDefaultRuntimeRestoreIsOneShotAndOrdered(t *testing.T) {
	t.Cleanup(func() {
		SetDefaultRuntime(nil)
	})

	firstRecorder := tracetest.NewSpanRecorder()
	firstTraceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(firstRecorder))
	t.Cleanup(func() {
		require.NoError(t, firstTraceProvider.Shutdown(context.Background()))
	})

	first := &Runtime{
		otel: &otelRuntime{
			tracer: firstTraceProvider.Tracer(telemetryScopeName),
		},
	}
	restoreFirst := SetDefaultRuntime(first)

	secondRecorder := tracetest.NewSpanRecorder()
	secondTraceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(secondRecorder))
	t.Cleanup(func() {
		require.NoError(t, secondTraceProvider.Shutdown(context.Background()))
	})

	second := &Runtime{
		otel: &otelRuntime{
			tracer: secondTraceProvider.Tracer(telemetryScopeName),
		},
	}
	restoreSecond := SetDefaultRuntime(second)

	restoreFirst()
	_, span := StartDefaultSpan(context.Background(), "still_second")
	span.End()
	require.Len(t, secondRecorder.Ended(), 1)
	require.Empty(t, firstRecorder.Ended())

	restoreSecond()
	_, span = StartDefaultSpan(context.Background(), "restored_to_first")
	span.End()
	require.Len(t, firstRecorder.Ended(), 1)
	require.Len(t, secondRecorder.Ended(), 1)
	assert.Equal(t, "restored_to_first", firstRecorder.Ended()[0].Name())

	restoreSecond()
	_, span = StartDefaultSpan(context.Background(), "still_first")
	span.End()
	require.Len(t, firstRecorder.Ended(), 2)
	require.Len(t, secondRecorder.Ended(), 1)
	assert.Equal(t, "still_first", firstRecorder.Ended()[1].Name())

	restoreFirst()
	_, span = StartDefaultSpan(context.Background(), "still_first_after_double_restore")
	span.End()
	require.Len(t, firstRecorder.Ended(), 3)
	require.Len(t, secondRecorder.Ended(), 1)
	assert.Equal(t, "still_first_after_double_restore", firstRecorder.Ended()[2].Name())
}

func TestStartDefaultSpanWithoutRuntimeDoesNotEndParentSpan(t *testing.T) {
	t.Cleanup(SetDefaultRuntime(nil))

	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})

	parentCtx, parentSpan := traceProvider.Tracer("runtime-test").Start(context.Background(), "parent")

	ctx, span := StartDefaultSpan(parentCtx, "child")

	assert.Same(t, parentCtx, ctx)
	assert.Equal(t, trace.SpanContextFromContext(parentCtx), trace.SpanContextFromContext(ctx))

	span.End()
	require.Empty(t, recorder.Ended(), "fallback span end should not end the parent span")

	parentSpan.End()
	require.Len(t, recorder.Ended(), 1)
	assert.Equal(t, "parent", recorder.Ended()[0].Name())
}

func TestRuntimeStartSpanWithoutOTelDoesNotEndParentSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})

	parentCtx, parentSpan := traceProvider.Tracer("runtime-test").Start(context.Background(), "parent")

	ctx, span := (&Runtime{}).StartSpan(parentCtx, "child")

	assert.Same(t, parentCtx, ctx)
	assert.Equal(t, trace.SpanContextFromContext(parentCtx), trace.SpanContextFromContext(ctx))

	span.End()
	require.Empty(t, recorder.Ended(), "fallback span end should not end the parent span")

	parentSpan.End()
	require.Len(t, recorder.Ended(), 1)
	assert.Equal(t, "parent", recorder.Ended()[0].Name())
}

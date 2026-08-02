package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestStartOTelMergesDefaultResourceWithoutSchemaConflict(t *testing.T) {
	runtime, err := startOTel(context.Background(), OTelConfig{
		Endpoint:    "localhost:4318",
		Insecure:    true,
		ServiceName: "heimdallr",
		Environment: "test",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtime.Shutdown(context.Background()))
	})
}

func TestStartOTelExportsLogsAndTracesOverHTTP(t *testing.T) {
	var logRequests atomic.Int64
	var traceRequests atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch r.URL.Path {
		case "/i/v1/logs":
			logRequests.Add(1)
		case "/i/v1/traces":
			traceRequests.Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)

	runtime, err := startOTel(context.Background(), OTelConfig{
		Endpoint:    collector.URL + "/i/v1/logs",
		ServiceName: "heimdallr",
		Environment: "test",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtime.Shutdown(context.Background()))
	})

	slog.New(runtime.logHandler).InfoContext(context.Background(), "http log")
	_, span := runtime.StartSpan(context.Background(), "http trace")
	span.End()

	require.NoError(t, runtime.logProvider.ForceFlush(context.Background()))
	require.NoError(t, runtime.traceProvider.ForceFlush(context.Background()))
	assert.GreaterOrEqual(t, logRequests.Load(), int64(1))
	assert.GreaterOrEqual(t, traceRequests.Load(), int64(1))
}

func TestOTelHTTPEndpointNormalizesSignalSpecificURL(t *testing.T) {
	endpoint := otelHTTPEndpointFromConfig(OTelConfig{
		Endpoint: "https://collector.example.com/i/v1/logs",
	})

	assert.Equal(t, "collector.example.com", endpoint.endpoint)
	assert.Equal(t, "/i/v1/logs", endpoint.signalPath(otelHTTPLogsPath))
	assert.Equal(t, "/i/v1/traces", endpoint.signalPath(otelHTTPTracesPath))
}

func TestOTelRuntimeWrapHTTPHandlerDoesNotExportClientIPs(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})

	runtime := &otelRuntime{traceProvider: traceProvider}
	handler := runtime.WrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/guild/123", nil)
	req.RemoteAddr = "203.0.113.10:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 198.51.100.8")
	req.Header.Set("Forwarded", "for=192.0.2.44;proto=https")
	req.Header.Set("X-Real-IP", "198.51.100.9")
	req.Header.Set("True-Client-IP", "198.51.100.10")
	req.Header.Set("CF-Connecting-IP", "198.51.100.11")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)

	attrs := spanAttributeValues(spans[0].Attributes())
	assert.NotContains(t, attrs, "network.peer.address")
	assert.NotContains(t, attrs, "client.address")

	serializedAttrs := fmt.Sprint(attrs)
	assert.NotContains(t, serializedAttrs, "203.0.113.10")
	assert.NotContains(t, serializedAttrs, "198.51.100.7")
	assert.NotContains(t, serializedAttrs, "198.51.100.8")
}

func TestOTelRuntimeWrapHTTPHandlerPreservesOriginalRequestForHandler(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})

	runtime := &otelRuntime{traceProvider: traceProvider}

	var gotRemoteAddr string
	var gotForwardedFor string
	var gotSpanContext trace.SpanContext

	handler := runtime.WrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRemoteAddr = r.RemoteAddr
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		gotSpanContext = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/guild/123", nil)
	req.RemoteAddr = "203.0.113.10:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 198.51.100.8")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.10:4567", gotRemoteAddr)
	assert.Equal(t, "198.51.100.7, 198.51.100.8", gotForwardedFor)
	assert.True(t, gotSpanContext.IsValid())
}

func TestOTelRuntimeWrapHTTPHandlerExportsSanitizedRouteWithoutUserAgent(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})

	runtime := &otelRuntime{traceProvider: traceProvider}
	handler := runtime.WrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://example.com/guild/123/posts/456?code=secret", nil)
	req.Header.Set("User-Agent", "SensitiveBrowser/99.0")
	req.Header.Set("Sec-CH-UA", "\"Chromium\";v=\"126\"")
	req.Header.Set("Sec-CH-UA-Platform", "\"macOS\"")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)

	attrs := spanAttributeValues(spans[0].Attributes())
	assert.Equal(t, "POST /guild/{id}/posts/{id}", spans[0].Name())
	assert.Equal(t, "/guild/{id}/posts/{id}", attrs["url.path"])

	serializedAttrs := fmt.Sprint(attrs)
	assert.NotContains(t, serializedAttrs, "/guild/123/posts/456")
	assert.NotContains(t, serializedAttrs, "code=secret")
	assert.NotContains(t, serializedAttrs, "SensitiveBrowser/99.0")
	assert.NotContains(t, serializedAttrs, "Chromium")
	assert.NotContains(t, attrs, "user_agent.original")
}

func TestBuildOTelLogHandlerRedactsOperationalKeysBeforeExport(t *testing.T) {
	exporter := &recordingLogExporter{}
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	t.Cleanup(func() {
		require.NoError(t, logProvider.Shutdown(context.Background()))
	})

	logger := slog.New(buildOTelLogHandler(logProvider))
	logger.ErrorContext(context.Background(), "runtime failure",
		"err", "boom detail",
		"error", "raw error prose",
		slog.Group("telemetry",
			slog.Group("crash",
				slog.String("panic", "panic payload"),
				slog.String("stack", "frame1\nframe2"),
				slog.String("outcome", "retryable"),
			),
			slog.String("error_count", "2"),
			slog.Bool("panic_recovered", true),
			slog.String("guild_id", "123"),
			slog.String("authorization", "Bearer secret"),
		),
	)

	require.NoError(t, logProvider.ForceFlush(context.Background()))
	require.Len(t, exporter.records, 1)

	attrs := flattenLogRecordAttributes(exporter.records[0])
	assert.Equal(t, redactedValue, attrs["err"].AsString())
	assert.Equal(t, redactedValue, attrs["error"].AsString())
	assert.Equal(t, redactedValue, attrs["telemetry.crash.panic"].AsString())
	assert.Equal(t, redactedValue, attrs["telemetry.crash.stack"].AsString())
	assert.Equal(t, "retryable", attrs["telemetry.crash.outcome"].AsString())
	assert.Equal(t, "2", attrs["telemetry.error_count"].AsString())
	assert.True(t, attrs["telemetry.panic_recovered"].AsBool())
	assert.Equal(t, "123", attrs["telemetry.guild_id"].AsString())
	assert.Equal(t, redactedValue, attrs["telemetry.authorization"].AsString())
}

func spanAttributeValues(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = fmt.Sprint(attr.Value.AsInterface())
	}
	return values
}

type recordingLogExporter struct {
	records []sdklog.Record
}

func (e *recordingLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

func (e *recordingLogExporter) Shutdown(context.Context) error {
	return nil
}

func (e *recordingLogExporter) ForceFlush(context.Context) error {
	return nil
}

func flattenLogRecordAttributes(record sdklog.Record) map[string]otellog.Value {
	attrs := make(map[string]otellog.Value, record.AttributesLen())
	record.WalkAttributes(func(attr otellog.KeyValue) bool {
		flattenLogValue(attrs, attr.Key, attr.Value)
		return true
	})
	return attrs
}

func flattenLogValue(dst map[string]otellog.Value, key string, value otellog.Value) {
	if value.Kind() != otellog.KindMap {
		dst[key] = value
		return
	}
	for _, child := range value.AsMap() {
		flattenLogValue(dst, key+"."+child.Key, child.Value)
	}
}

package web

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"

	"github.com/NLLCommunity/heimdallr/model"
)

func TestTelemetryRequestLogMiddlewareLogsStatusDurationAndPath(t *testing.T) {
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(original) })

	mux := http.NewServeMux()
	mux.HandleFunc("POST /guild/{id}/posts/{postID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	req := httptest.NewRequest(http.MethodPost, "/guild/123/posts/456?code=secret", nil)

	telemetryRequestLogMiddleware(mux).ServeHTTP(httptest.NewRecorder(), req)

	got := buf.String()
	assert.Contains(t, got, "http request completed")
	assert.Contains(t, got, "method=POST")
	assert.Contains(t, got, "path=/guild/{id}/posts/{id}")
	assert.Contains(t, got, "status=201")
	assert.Contains(t, got, "authenticated=false")
	assert.Contains(t, got, "duration=")
	assert.NotContains(t, got, "guild_id=123")
	assert.NotContains(t, got, "code=secret")
	assert.NotContains(t, got, "/guild/123/posts/456")
}

func TestTelemetryRequestLogMiddlewareDefaultsStatusTo200(t *testing.T) {
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	req := httptest.NewRequest(http.MethodGet, "/guilds", nil)

	telemetryRequestLogMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)

	assert.Contains(t, buf.String(), "status=200")
}

func TestTelemetryRequestLogMiddlewareLogsOnlySafeSessionFields(t *testing.T) {
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/guild/456", nil)
	req = req.WithContext(setSession(req.Context(), &model.DashboardSession{
		UserID:   snowflake.ID(42),
		Username: "secret-name",
		Token:    "secret-token",
	}))

	telemetryRequestLogMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)

	got := buf.String()
	assert.Contains(t, got, "authenticated=true")
	assert.Contains(t, got, "user_id=42")
	assert.NotContains(t, got, "secret-name")
	assert.NotContains(t, got, "secret-token")
}

func TestStatusRecorderUnwrapReturnsUnderlyingWriter(t *testing.T) {
	base := httptest.NewRecorder()
	recorder := &statusRecorder{ResponseWriter: base}

	assert.Same(t, base, recorder.Unwrap())
}

func TestStatusRecorderPreservesFlusherViaResponseController(t *testing.T) {
	base := &flushableResponseWriter{ResponseWriter: httptest.NewRecorder()}
	recorder := &statusRecorder{ResponseWriter: base}

	err := http.NewResponseController(recorder).Flush()

	assert.NoError(t, err)
	assert.True(t, base.flushed)
}

func TestStatusRecorderPreservesFlusherInterface(t *testing.T) {
	base := &flushableResponseWriter{ResponseWriter: httptest.NewRecorder()}
	recorder := &statusRecorder{ResponseWriter: base}

	flusher, ok := any(recorder).(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder should preserve http.Flusher")
	}

	flusher.Flush()

	assert.True(t, base.flushed)
}

type flushableResponseWriter struct {
	http.ResponseWriter
	flushed bool
}

func (w *flushableResponseWriter) Flush() {
	w.flushed = true
}

var errHijackNotSupported = errors.New("hijack not supported in test")

func (w *flushableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errHijackNotSupported
}

func (w *flushableResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(w.ResponseWriter, r)
}

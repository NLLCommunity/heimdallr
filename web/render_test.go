package web

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// staticComponent is a templ.Component fixture that emits a fixed HTML string
// without any rendering machinery — keeps these tests focused on header/status
// behavior rather than templ internals.
type staticComponent string

func (s staticComponent) Render(_ context.Context, w io.Writer) error {
	_, err := io.WriteString(w, string(s))
	return err
}

type errComponent struct {
	err error
}

func (c errComponent) Render(context.Context, io.Writer) error {
	return c.err
}

type failingResponseWriter struct {
	header http.Header
	status int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRenderSafe_DefaultsTo200WithHTMLContentType(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	renderSafe(rec, req, staticComponent("<p>ok</p>"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "<p>ok</p>", rec.Body.String())
}

// HTMX-aware error responses need both 4xx/5xx status AND text/html so the
// client-side swap filter (htmx-config.js: `code:'[45]..',swap:true,error:true`
// + the beforeSwap text/html guard) lands the AlertError partial inline.
// Order matters: Content-Type must be set before WriteHeader, or the header
// is dropped from the response.
func TestRenderSafeStatus_SetsContentTypeBeforeStatus(t *testing.T) {
	cases := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusRequestEntityTooLarge,
		http.StatusTooManyRequests,
		http.StatusBadGateway,
	}
	for _, status := range cases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			req := httptest.NewRequest("POST", "/x", nil)
			rec := httptest.NewRecorder()

			renderSafeStatus(rec, req, status, staticComponent(`<div class="alert">err</div>`))

			assert.Equal(t, status, rec.Code)
			ct := rec.Header().Get("Content-Type")
			assert.True(t, strings.HasPrefix(ct, "text/html"),
				"Content-Type must be text/html for HTMX to swap; got %q", ct)
			assert.Contains(t, rec.Body.String(), "err")
		})
	}
}

func TestRenderSafeStatusRenderErrorLogsSanitizedRoute(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	req := httptest.NewRequest(http.MethodGet, "/guild/123/posts/456?token=secret", nil)
	rec := httptest.NewRecorder()

	renderSafeStatus(rec, req, http.StatusOK, errComponent{err: errors.New("render failed")})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	output := buf.String()
	assert.Contains(t, output, "failed to render template")
	assert.Contains(t, output, "path=/guild/{id}/posts/{id}")
	assert.NotContains(t, output, "/guild/123/posts/456")
	assert.NotContains(t, output, "token=secret")
}

func TestRenderSafeStatusWriteErrorLogsSanitizedRoute(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	req := httptest.NewRequest(http.MethodGet, "/guild/123/posts/456?token=secret", nil)
	w := &failingResponseWriter{}

	renderSafeStatus(w, req, http.StatusAccepted, staticComponent("<p>ok</p>"))

	assert.Equal(t, http.StatusAccepted, w.status)

	output := buf.String()
	assert.Contains(t, output, "failed to write response")
	assert.Contains(t, output, "path=/guild/{id}/posts/{id}")
	assert.NotContains(t, output, "/guild/123/posts/456")
	assert.NotContains(t, output, "token=secret")
}

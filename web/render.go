package web

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
)

// renderSafe renders a templ component into a buffer first so that a render
// failure can be turned into a 500 response. Without buffering, templ would
// write incrementally and commit the 200 status before any error surfaces.
//
// Content-Type is set explicitly so the client-side HTMX swap filter (which
// rejects non-HTML error bodies) can rely on the header instead of MIME
// sniffing.
func renderSafe(w http.ResponseWriter, r *http.Request, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		slog.Error("failed to render template", "error", err, "path", r.URL.Path)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("failed to write response", "error", err, "path", r.URL.Path)
	}
}

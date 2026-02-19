package rpcserver

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:frontend
var frontendFS embed.FS

// spaHandler serves the embedded frontend files, falling back to index.html
// for any path that doesn't match a real file (SPA client-side routing).
type spaHandler struct {
	fs http.Handler
	root fs.FS
}

func newSPAHandler() *spaHandler {
	sub, _ := fs.Sub(frontendFS, "frontend")
	return &spaHandler{
		fs:   http.FileServerFS(sub),
		root: sub,
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to serve the requested file.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	// Check if the file exists in the embedded FS.
	if _, err := fs.Stat(h.root, path); err == nil {
		// Set long cache for hashed assets.
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		h.fs.ServeHTTP(w, r)
		return
	}

	// File not found — serve index.html for SPA fallback.
	r.URL.Path = "/"
	h.fs.ServeHTTP(w, r)
}

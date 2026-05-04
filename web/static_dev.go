//go:build dev

package web

import "net/http"

// Dev-mode static FS. The path is cwd-relative, so the binary must be run
// from the repo root for assets to resolve — `go run .`, `air`, and `task`
// all satisfy that. Prod builds use the embed.FS in static_prod.go and
// don't have this constraint.
func getStaticFS() http.FileSystem {
	return http.Dir("./web/static")
}

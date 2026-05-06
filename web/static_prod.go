//go:build !dev

package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:static
var staticFS embed.FS

func getStaticFS() http.FileSystem {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("Failed to get embedded static files: " + err.Error())
	}
	return http.FS(sub)
}

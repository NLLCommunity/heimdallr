//go:build dev

package web

import "net/http"

func getStaticFS() http.FileSystem {
	return http.Dir("./web/static")
}

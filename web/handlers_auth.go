package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

// canonicalOrigin returns scheme://host with the default port for the
// scheme stripped, matching how browsers serialize the Origin header.
// The CORS middleware uses this so a base_url like "https://example.com:443"
// matches a browser-sent Origin of "https://example.com".
func canonicalOrigin(u *url.URL) string {
	host := u.Host
	switch {
	case u.Scheme == "https" && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	case u.Scheme == "http" && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	}
	return u.Scheme + "://" + host
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to guilds.
	if session := sessionFromContext(r.Context()); session != nil {
		http.Redirect(w, r, "/guilds", http.StatusSeeOther)
		return
	}
	renderSafe(w, r, pages.Login())
}

func handleLogout(secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// DeleteSession expects the raw cookie token (it hashes internally to
		// look up the row). The session in context comes from GetSession, whose
		// Token field holds the DB-stored hash — passing that here would hash
		// twice and silently fail to delete the row.
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			if err := model.DeleteSession(cookie.Value); err != nil {
				slog.Error("failed to delete session", "error", err)
			}
		}

		// MaxAge=-1 emits `Max-Age: 0`, instructing the browser to delete the
		// cookie immediately. MaxAge=0 omits the attribute, leaving the cookie
		// as a session cookie that some browsers persist via tab restore.
		http.SetCookie(w, makeSessionCookie("", -1, secureCookie))
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// authMiddleware enforces auth on `/`, so by the time we get here the
	// session is guaranteed to be present — unauthenticated requests were
	// already redirected to the login destination by the middleware.
	http.Redirect(w, r, "/guilds", http.StatusSeeOther)
}

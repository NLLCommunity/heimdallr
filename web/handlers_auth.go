package web

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

// handleCallbackGET renders a confirmation page without consuming the login code.
// This prevents Discord link previews and crawlers from exchanging the code.
func handleCallbackGET(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	renderSafe(w, r, pages.Callback(code))
}

// handleCallbackPOST exchanges the login code for a session. The Origin/Referer
// header is checked against the configured dashboard origin to block login-CSRF
// (a malicious site auto-submitting an attacker-generated code to log the
// victim into the attacker's account).
func handleCallbackPOST(allowedOrigin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isSameOriginPost(r, allowedOrigin) {
			slog.Warn(
				"rejecting cross-origin POST /callback",
				"origin", r.Header.Get("Origin"),
				"referer", r.Header.Get("Referer"),
			)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		code := r.FormValue("code")
		if code == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := model.ExchangeLoginCode(code)
		if err != nil {
			slog.Warn("invalid login code exchange", "error", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		http.SetCookie(w, makeSessionCookie(session.Token, int(model.SessionExpiry.Seconds())))
		http.Redirect(w, r, "/guilds", http.StatusSeeOther)
	}
}

// isSameOriginPost reports whether r appears to come from the configured
// dashboard origin. Modern browsers attach `Origin` to every cross-site or
// state-changing request, so a missing or mismatched header indicates a
// cross-origin (or non-browser) submission. `Referer` is used as a fallback
// for the rare clients that omit Origin. Both missing => reject.
func isSameOriginPost(r *http.Request, allowedOrigin string) bool {
	if got := r.Header.Get("Origin"); got != "" {
		return got == allowedOrigin
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		return u.Scheme+"://"+u.Host == allowedOrigin
	}
	return false
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to guilds.
	if session := sessionFromContext(r.Context()); session != nil {
		http.Redirect(w, r, "/guilds", http.StatusSeeOther)
		return
	}
	renderSafe(w, r, pages.Login())
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	// DeleteSession expects the raw cookie token (it hashes internally to look
	// up the row). The session in context comes from GetSession, whose Token
	// field holds the DB-stored hash — passing that here would hash twice and
	// silently fail to delete the row.
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := model.DeleteSession(cookie.Value); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}

	// MaxAge=-1 emits `Max-Age: 0`, instructing the browser to delete the
	// cookie immediately. MaxAge=0 omits the attribute, leaving the cookie as
	// a session cookie that some browsers persist via tab restore.
	http.SetCookie(w, makeSessionCookie("", -1))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if session := sessionFromContext(r.Context()); session != nil {
		http.Redirect(w, r, "/guilds", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

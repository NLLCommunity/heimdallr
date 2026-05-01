package web

import (
	"log/slog"
	"net/http"

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

// handleCallbackPOST exchanges the login code for a session.
func handleCallbackPOST(w http.ResponseWriter, r *http.Request) {
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

	http.SetCookie(w, makeSessionCookie(session.Token, 86400))
	http.Redirect(w, r, "/guilds", http.StatusSeeOther)
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
	session := sessionFromContext(r.Context())
	if session != nil {
		if err := model.DeleteSession(session.Token); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}

	http.SetCookie(w, makeSessionCookie("", 0))
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

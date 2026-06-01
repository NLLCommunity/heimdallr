package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
)

// oauthScopes are the OAuth2 scopes we request from Discord:
//   - identify: required to call GetCurrentUser and bind the session to a
//     Discord user ID after the code exchange.
//   - guilds: required to call GetCurrentUserGuilds — without this scope
//     /guilds can't list a user's admin servers.
var oauthScopes = []discord.OAuth2Scope{
	discord.OAuth2ScopeIdentify,
	discord.OAuth2ScopeGuilds,
}

// oauthStateCookieName is the short-lived cookie that pins the OAuth
// state to the user agent. /oauth/start sets it; /oauth/callback compares
// its value to the state query parameter and refuses any mismatch. Without
// this, an attacker could craft an authorize URL and email it to a victim
// to log the victim into the attacker's account (login-CSRF).
const oauthStateCookieName = "heimdallr_oauth_state"

// oauthRedirectURI returns the absolute redirect URI Discord sends the
// user back to after consent. Built from dashboard.base_url +
// "/oauth/callback"; must match a redirect URI configured on the
// application's Discord developer portal page.
func oauthRedirectURI(base *url.URL) string {
	u := *base
	u.Path = "/oauth/callback"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// buildAuthorizeURL constructs the Discord OAuth2 authorize URL with the
// given state. We use discord.AuthorizeURL rather than rolling our own
// formatter so any future query-key change in disgo flows through without
// a code change here.
func buildAuthorizeURL(clientID snowflake.ID, redirectURI, state string) string {
	return discord.AuthorizeURL(discord.QueryValues{
		"client_id":     clientID,
		"redirect_uri":  redirectURI,
		"response_type": "code",
		"scope":         discord.JoinScopes(oauthScopes),
		"state":         state,
	})
}

// safeReturnTo validates and normalizes a return-to URL submitted via
// query parameter. Returns the canonical relative path or "" if the
// input is unsafe (absolute URL, scheme/host present, missing /guild/
// prefix, or contains characters that the path-matcher in the auth
// middleware can't sanitize).
//
// The allowlist is intentionally narrow: every deep-link we generate is
// of the form "/guild/{snowflake}[/sub-path]". A wider allowlist would
// expose an open-redirect for any path the dashboard serves, including
// /static/, which can be arbitrary content.
func safeReturnTo(raw string) string {
	if raw == "" {
		return ""
	}
	// url.Parse with an absolute or scheme-bearing URL still parses but
	// leaves Scheme/Host populated — both unsafe.
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "" || u.Host != "" {
		return ""
	}
	// Reject control characters that could split headers before they
	// reach path.Clean (which preserves them).
	if strings.ContainsAny(u.Path, "\x00\r\n") {
		return ""
	}
	// path.Clean collapses "..", ".", and double-slash segments so a
	// crafted input like "/guild/../login" becomes "/login" and fails
	// the prefix check below. Without this, the browser would resolve
	// the traversal client-side and land the user on a path the
	// allowlist was meant to exclude — still same-origin, but not what
	// we promised the caller.
	cleaned := path.Clean(u.Path)
	if !strings.HasPrefix(cleaned, "/guild/") {
		return ""
	}
	// Re-encode to drop fragments and normalize separators.
	out := cleaned
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

// handleOAuthStart kicks off the OAuth handshake. Public route — no
// session required, since this is also the entry point for first-time
// logins from /login. Stores a fresh state row and an oauth-state cookie
// matching it, then redirects to Discord.
func handleOAuthStart(clientID snowflake.ID, redirectURI string, secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
		state, err := model.CreateOAuthState(returnTo)
		if err != nil {
			slog.Error("oauth: failed to create state row", "err", err)
			http.Error(w, "failed to start login", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     oauthStateCookieName,
			Value:    state,
			Path:     "/oauth/",
			MaxAge:   600,
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, buildAuthorizeURL(clientID, redirectURI, state), http.StatusSeeOther)
	}
}

// handleOAuthCallback finishes the Discord OAuth dance: validates the
// state row AND the matching oauth-state cookie, exchanges the code for
// an access/refresh token pair, fetches the user via /users/@me to bind
// the session to a Discord user ID, then mints an admin session.
//
// The dual state check — DB row plus cookie — defends against login
// CSRF. The DB row alone proves the state was issued by us; the cookie
// proves it was issued to *this* browser.
func handleOAuthCallback(
	client *bot.Client,
	clientID snowflake.ID,
	clientSecret string,
	redirectURI string,
	crypto *model.TokenCrypto,
	secureCookie bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Always clear the state cookie before returning, success or
		// failure — never let it survive the callback.
		defer http.SetCookie(w, &http.Cookie{
			Name:     oauthStateCookieName,
			Value:    "",
			Path:     "/oauth/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
		})

		// Discord sends ?error=access_denied&error_description=... when
		// the user clicks "Cancel" on consent. Treat as a normal bounce.
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			slog.Info("oauth: user cancelled consent",
				"error", errParam,
				"description", r.URL.Query().Get("error_description"),
			)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if code == "" || state == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		cookieState, err := r.Cookie(oauthStateCookieName)
		if err != nil || cookieState.Value == "" || cookieState.Value != state {
			slog.Warn("oauth: state cookie mismatch on callback")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		stateRow, err := model.ConsumeOAuthState(state)
		if err != nil {
			slog.Warn("oauth: invalid state on callback", "err", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		tok, err := client.Rest.GetAccessToken(clientID, clientSecret, code, redirectURI)
		if err != nil {
			slog.Warn("oauth: token exchange failed", "err", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		me, err := client.Rest.GetCurrentUser(tok.AccessToken)
		if err != nil {
			slog.Warn("oauth: GetCurrentUser failed after token exchange", "err", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		sealedAccess, err := crypto.Seal(tok.AccessToken)
		if err != nil {
			slog.Error("oauth: seal access token failed", "err", err)
			http.Error(w, "failed to finish login", http.StatusInternalServerError)
			return
		}
		sealedRefresh, err := crypto.Seal(tok.RefreshToken)
		if err != nil {
			slog.Error("oauth: seal refresh token failed", "err", err)
			http.Error(w, "failed to finish login", http.StatusInternalServerError)
			return
		}

		avatar := ""
		if me.Avatar != nil {
			avatar = *me.Avatar
		}
		newSession, err := model.CreateAdminSession(
			model.SessionIdentity{
				UserID:   me.ID,
				Username: me.Username,
				Avatar:   avatar,
			},
			sealedAccess,
			sealedRefresh,
			time.Now().Add(tok.ExpiresIn),
		)
		if err != nil {
			slog.Error("oauth: failed to create admin session", "err", err)
			http.Error(w, "failed to finish login", http.StatusInternalServerError)
			return
		}

		maxAge := int(time.Until(newSession.ExpiresAt).Seconds())
		http.SetCookie(w, makeSessionCookie(newSession.Token, maxAge, secureCookie))

		dest := stateRow.ReturnTo
		if dest == "" {
			dest = "/guilds"
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
	}
}

// freshAccessToken returns a usable plaintext access token for the given
// session, refreshing it via Discord if the stored token is expired.
//
// If refresh succeeds, the DB row is updated with the new tokens so
// subsequent requests don't pay the refresh round-trip. If refresh
// fails (revoked grant, network error), the caller is expected to
// redirect the user back through OAuth — there's no fall-back path
// because the entire point of the OAuth flow is user-scope access.
func freshAccessToken(client *bot.Client, clientID snowflake.ID, clientSecret string, crypto *model.TokenCrypto, session *model.DashboardSession) (string, error) {
	// 30s of slack so we don't hand out a token that expires mid-request.
	if time.Until(session.TokenExpiresAt) > 30*time.Second {
		return crypto.Open(session.AccessTokenEnc)
	}
	refreshToken, err := crypto.Open(session.RefreshTokenEnc)
	if err != nil {
		return "", err
	}
	tok, err := client.Rest.RefreshAccessToken(clientID, clientSecret, refreshToken)
	if err != nil {
		return "", err
	}
	sealedAccess, err := crypto.Seal(tok.AccessToken)
	if err != nil {
		return "", err
	}
	sealedRefresh, err := crypto.Seal(tok.RefreshToken)
	if err != nil {
		return "", err
	}
	newExpiry := time.Now().Add(tok.ExpiresIn)
	if err := model.UpdateSessionTokens(session.Token, sealedAccess, sealedRefresh, newExpiry); err != nil {
		return "", err
	}
	session.AccessTokenEnc = sealedAccess
	session.RefreshTokenEnc = sealedRefresh
	session.TokenExpiresAt = newExpiry
	return tok.AccessToken, nil
}

package web

import (
	"context"
	"net/http"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
)

type sessionContextKey struct{}

func sessionFromContext(ctx context.Context) *model.DashboardSession {
	session, _ := ctx.Value(sessionContextKey{}).(*model.DashboardSession)
	return session
}

func setSession(ctx context.Context, session *model.DashboardSession) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, session)
}

const sessionCookieName = "heimdallr_session"

func makeSessionCookie(token string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// isGuildAdmin checks whether a user has admin permission in a guild.
func isGuildAdmin(client *bot.Client, guild discord.Guild, userID snowflake.ID) bool {
	if guild.OwnerID == userID {
		return true
	}

	member, ok := client.Caches.Member(guild.ID, userID)
	if !ok {
		m, err := client.Rest.GetMember(guild.ID, userID)
		if err != nil {
			return false
		}
		member = *m
	}

	perms := client.Caches.MemberPermissions(member)
	return perms.Has(discord.PermissionAdministrator)
}

// checkGuildAdmin verifies the session user is an admin in the given guild.
// Returns the parsed guild ID and true on success, or writes an error response and returns false.
//
// Non-session error branches use http.Error (text/plain). HTMX's beforeSwap
// handler in htmx-config.js suppresses the swap for non-HTML responses and
// surfaces the body as a toast — so the form section stays usable and the
// user sees the actual reason. A text/html error partial would let HTMX
// replace the targeted section with just an alert div, which is worse UX.
func checkGuildAdmin(w http.ResponseWriter, r *http.Request, client *bot.Client, guildIDStr string) (snowflake.ID, bool) {
	session := sessionFromContext(r.Context())
	if session == nil {
		// HTMX silently swallows 3xx; redirectToLogin emits HX-Redirect for
		// HTMX requests so the browser actually navigates to /login.
		redirectToLogin(w, r)
		return 0, false
	}

	guildID, err := snowflake.Parse(guildIDStr)
	if err != nil {
		http.Error(w, "invalid guild ID", http.StatusBadRequest)
		return 0, false
	}

	guild, ok := client.Caches.Guild(guildID)
	if !ok {
		http.Error(w, "bot is not in this guild", http.StatusForbidden)
		return 0, false
	}

	if !isGuildAdmin(client, guild, session.UserID) {
		http.Error(w, "you do not have administrator permission in this guild", http.StatusForbidden)
		return 0, false
	}

	return guildID, true
}

// parseSnowflakeOrZero parses a snowflake ID, treating "" as an explicit
// "unset" (returns 0, nil). Returns an error for non-empty values that are
// not valid snowflakes — callers must surface this to the user instead of
// silently clearing the field.
func parseSnowflakeOrZero(s string) (snowflake.ID, error) {
	if s == "" {
		return 0, nil
	}
	return snowflake.Parse(s)
}

// idStr converts a snowflake ID to a string, returning "" for zero.
func idStr(id snowflake.ID) string {
	if id == 0 {
		return ""
	}
	return id.String()
}

// checkGuildPostMod verifies the session user can access the post dashboard
// for the given guild — i.e., they're an admin OR they pass the
// /post-dashboard command's permission check. Mirrors checkGuildAdmin's
// error semantics. The caller passes the cached command ID + default perm
// so this helper doesn't need a global registry of slash commands.
func checkGuildPostMod(w http.ResponseWriter, r *http.Request, client *bot.Client, guildIDStr string, postDashboardCmdID snowflake.ID, defaultPerm discord.Permissions) (snowflake.ID, bool) { //nolint:unused
	session := sessionFromContext(r.Context())
	if session == nil {
		redirectToLogin(w, r)
		return 0, false
	}

	guildID, err := snowflake.Parse(guildIDStr)
	if err != nil {
		http.Error(w, "invalid guild ID", http.StatusBadRequest)
		return 0, false
	}

	guild, ok := client.Caches.Guild(guildID)
	if !ok {
		http.Error(w, "bot is not in this guild", http.StatusForbidden)
		return 0, false
	}

	if !canUsePostDashboard(client, guild, session.UserID, postDashboardCmdID, defaultPerm) {
		http.Error(w, "you do not have permission to access the post dashboard in this guild", http.StatusForbidden)
		return 0, false
	}

	return guildID, true
}

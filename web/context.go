package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/spf13/viper"

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

func makeSessionCookie(token string, maxAge int) *http.Cookie {
	secure := strings.HasPrefix(viper.GetString("dashboard.base_url"), "https")
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
func checkGuildAdmin(w http.ResponseWriter, r *http.Request, client *bot.Client, guildIDStr string) (snowflake.ID, bool) {
	session := sessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
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

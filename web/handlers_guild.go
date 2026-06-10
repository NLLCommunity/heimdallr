package web

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

// isUnauthorizedRest reports whether err is a Discord REST 401, meaning
// the bearer token was revoked or invalidated upstream even though it
// has not expired locally.
func isUnauthorizedRest(err error) bool {
	var restErr *rest.Error
	return errors.As(err, &restErr) && restErr.Response != nil &&
		restErr.Response.StatusCode == http.StatusUnauthorized
}

// handleGuilds renders the multi-guild picker. The user's guild list comes
// from Discord's "Get Current User Guilds" endpoint via the OAuth bearer
// token — one REST call regardless of how many guilds the bot is in —
// intersected with the bot's own guild set so we only show servers the
// bot can actually render dashboards for.
//
// Tiles are labeled by access level:
//
//   - Admin: owner or PermissionAdministrator. Linked to /guild/{id}, the
//     full settings dashboard.
//   - Posts: holder of the guild's configured PostsModRoleID (only
//     evaluated for non-admins, and only for guilds where an admin has
//     opted in by setting the role). Linked directly to /guild/{id}/posts.
//
// The posts check is cheap when the user's member is cached, falls back
// to a single GetMember REST call otherwise. The fallback is bounded by
// "guilds where the bot is installed, the user isn't admin, and the
// posts role is configured" — typically a small set.
func handleGuilds(client *bot.Client, clientID snowflake.ID, clientSecret string, crypto *model.TokenCrypto) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		accessToken, err := freshAccessToken(client, clientID, clientSecret, crypto, session)
		if err != nil {
			// Most likely refresh-token revocation — Discord answers
			// 400 invalid_grant once the user removes the app from
			// their authorized list. Send them through consent again.
			slog.Info("guilds: freshAccessToken failed, redirecting through OAuth", "err", err)
			http.Redirect(w, r, "/oauth/start", http.StatusSeeOther)
			return
		}

		userGuilds, err := client.Rest.GetCurrentUserGuilds(accessToken, 0, 0, 0, false)
		if err != nil {
			// freshAccessToken only checks the local TokenExpiresAt, so a
			// token the user revoked upstream (Discord's Authorized Apps
			// page) still reaches this call and comes back 401. Without
			// this branch the user is stranded on a 502 on every visit
			// until local expiry; sending them through consent again is
			// the same recovery the refresh-failure path uses.
			if isUnauthorizedRest(err) {
				slog.Info("guilds: access token rejected by Discord, redirecting through OAuth", "err", err)
				http.Redirect(w, r, "/oauth/start", http.StatusSeeOther)
				return
			}
			slog.Warn("guilds: GetCurrentUserGuilds failed", "err", err)
			http.Error(w, "failed to load your servers", http.StatusBadGateway)
			return
		}

		// Intersect the user's guilds with the bot's via direct cache
		// lookups - the user holds at most 200 guilds, while snapshotting
		// the bot's entire guild cache would scale per-request work (and
		// allocations) with bot size instead. Stuck (unready/unavailable)
		// guilds still count as "bot is in the guild" but have no cached
		// metadata, so their tiles render whatever the OAuth2Guild
		// payload carried.
		var guilds []pages.GuildData
		for _, ug := range userGuilds {
			var cachedPtr *discord.Guild
			if cached, inCache := client.Caches.Guild(ug.ID); inCache {
				cachedPtr = &cached
			} else if !client.Caches.IsGuildUnready(ug.ID) && !client.Caches.IsGuildUnavailable(ug.ID) {
				// Not cached and not stuck: the bot is not in this guild.
				continue
			}

			role := resolveGuildRole(client, cachedPtr, ug, session.UserID)
			if role == "" {
				continue
			}

			name, icon := ug.Name, ug.Icon
			if cachedPtr != nil {
				name = cachedPtr.Name
				icon = cachedPtr.Icon
			}
			var iconStr string
			if icon != nil {
				iconStr = *icon
			}
			guilds = append(guilds, pages.GuildData{
				ID:   ug.ID.String(),
				Name: name,
				Icon: iconStr,
				Role: role,
			})
		}

		sort.Slice(guilds, func(i, j int) bool {
			ni, nj := strings.ToLower(guilds[i].Name), strings.ToLower(guilds[j].Name)
			if ni != nj {
				return ni < nj
			}
			return guilds[i].ID < guilds[j].ID
		})

		nav := layouts.NavData{User: session}
		renderSafe(w, r, pages.Guilds(nav, guilds))
	}
}

// resolveGuildRole decides how to label a tile for the given user in the
// given guild. Returns "" when the user has no access (don't show a tile
// at all).
//
// Admin/owner short-circuits at the top so neither GetGuildSettings nor
// GetMember runs in the common case. Posts-role evaluation requires the
// guild to be in the cache (we need cached.Guild to test admin via
// cached perms) — for stuck guilds we can't run the posts check, which
// is acceptable: it's the same population that the bot has temporarily
// lost gateway state for, and the user can re-navigate after Disgo
// recovers.
func resolveGuildRole(client *bot.Client, cached *discord.Guild, ug discord.OAuth2Guild, userID snowflake.ID) pages.GuildRole {
	if ug.Owner || ug.Permissions.Has(discord.PermissionAdministrator) {
		return pages.GuildRoleAdmin
	}
	if cached == nil {
		return ""
	}
	settings, err := model.GetGuildSettings(ug.ID)
	if err != nil || settings.PostsModRoleID == 0 {
		return ""
	}
	member := guildMember(client, ug.ID, userID)
	if member == nil {
		return ""
	}
	switch memberAccessLevel(client, *cached, member, settings.PostsModRoleID) {
	case guildAccessAdmin:
		return pages.GuildRoleAdmin
	case guildAccessPosts:
		return pages.GuildRolePosts
	}
	return ""
}

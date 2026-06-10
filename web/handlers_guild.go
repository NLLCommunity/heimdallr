package web

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

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
			slog.Warn("guilds: GetCurrentUserGuilds failed", "err", err)
			http.Error(w, "failed to load your servers", http.StatusBadGateway)
			return
		}

		// Snapshot the bot's guild set + cached guild metadata. Stuck
		// guild IDs go into the set too so they can still be intersected,
		// but they have no cached name/icon so the tile renders whatever
		// the OAuth2Guild payload carried.
		type cachedGuild struct {
			Guild *discord.Guild
		}
		botGuilds := make(map[snowflake.ID]cachedGuild)
		for g := range client.Caches.Guilds() {
			gCopy := g
			botGuilds[g.ID] = cachedGuild{Guild: &gCopy}
		}
		for _, id := range client.Caches.UnreadyGuildIDs() {
			if _, ok := botGuilds[id]; !ok {
				botGuilds[id] = cachedGuild{}
			}
		}
		for _, id := range client.Caches.UnavailableGuildIDs() {
			if _, ok := botGuilds[id]; !ok {
				botGuilds[id] = cachedGuild{}
			}
		}

		var guilds []pages.GuildData
		for _, ug := range userGuilds {
			cg, inBot := botGuilds[ug.ID]
			if !inBot {
				continue
			}

			role := resolveGuildRole(client, cg.Guild, ug, session.UserID)
			if role == "" {
				continue
			}

			name, icon := ug.Name, ug.Icon
			if cg.Guild != nil {
				name = cg.Guild.Name
				icon = cg.Guild.Icon
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

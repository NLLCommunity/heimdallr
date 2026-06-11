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
// The posts check is cache-only: a cold member cache hides the tile for
// that load rather than paying a REST GetMember per guild, and the
// per-guild gates still resolve access fully on direct navigation.
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

		var guilds []pages.GuildData
		addTile := func(ug discord.OAuth2Guild, cached *discord.Guild, role pages.GuildRole) {
			name, icon := ug.Name, ug.Icon
			if cached != nil {
				name, icon = cached.Name, cached.Icon
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

		// First pass: intersect the user's guilds with the bot's via
		// direct cache lookups - the user holds at most 200 guilds,
		// while snapshotting the bot's entire guild cache would scale
		// per-request work with bot size instead. Admin tiles resolve
		// straight from the OAuth payload; non-admin guilds become
		// posts-role candidates for the batched settings lookup below.
		type postsCandidate struct {
			ug     discord.OAuth2Guild
			cached discord.Guild
		}
		var candidates []postsCandidate
		var candidateIDs []snowflake.ID
		for _, ug := range userGuilds {
			isAdmin := ug.Owner || ug.Permissions.Has(discord.PermissionAdministrator)
			cached, inCache := client.Caches.Guild(ug.ID)
			if !inCache {
				if !client.Caches.IsGuildUnready(ug.ID) && !client.Caches.IsGuildUnavailable(ug.ID) {
					// Not cached and not stuck: the bot is not in this guild.
					continue
				}
				// Stuck (unready/unavailable) guild: the posts check
				// needs cached state, so only admins get a tile - and
				// only after REST-verifying membership, since the stuck
				// sets can keep holding guilds the bot was kicked from
				// while gateway state is stale. Bounded by the user's
				// stuck admin guilds, normally zero. The tile renders
				// whatever the OAuth2Guild payload carried.
				if !isAdmin {
					continue
				}
				if _, err := client.Rest.GetGuild(ug.ID, false); err != nil {
					continue
				}
				addTile(ug, nil, pages.GuildRoleAdmin)
				continue
			}
			if isAdmin {
				addTile(ug, &cached, pages.GuildRoleAdmin)
				continue
			}
			candidates = append(candidates, postsCandidate{ug: ug, cached: cached})
			candidateIDs = append(candidateIDs, ug.ID)
		}

		// One read-only query answers "which of these guilds configured
		// a posts-mod role" for the whole page, instead of a per-guild
		// FirstOrCreate that both round-trips N times and inserts empty
		// settings rows for unconfigured guilds on a pure read path.
		postsRoles, err := model.GetPostsModRoles(candidateIDs)
		if err != nil {
			// Degrade to admin-only tiles rather than failing the page.
			slog.Warn("guilds: failed to load posts-mod roles", "err", err)
		}
		for _, c := range candidates {
			roleID, ok := postsRoles[c.ug.ID]
			if !ok {
				continue
			}
			// Cache-only member lookup: a REST fallback here would fan
			// out into N sequential GetMember calls under the bot's
			// shared global rate limit whenever member caches are cold,
			// degrading the bot's moderation features just because users
			// refreshed the picker. On a miss the guild simply shows no
			// tile this load; the per-guild gates (checkGuildPostMod)
			// still do the full lookup when the user navigates directly.
			member, ok := client.Caches.Member(c.ug.ID, session.UserID)
			if !ok {
				continue
			}
			switch memberAccessLevel(client, c.cached, &member, roleID) {
			case guildAccessAdmin:
				addTile(c.ug, &c.cached, pages.GuildRoleAdmin)
			case guildAccessPosts:
				addTile(c.ug, &c.cached, pages.GuildRolePosts)
			}
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

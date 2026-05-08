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

	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

// handleGuilds renders the multi-guild picker that admins land on after
// /admin-dashboard. It iterates every guild the bot is in, which makes any
// per-guild Discord REST call a stampede risk for users in many guilds.
//
// Cost-shaping decisions:
//
//   - Only admin guilds are listed. Post-mods reach their dashboard via the
//     /post-dashboard slash command, which generates a login link that
//     bypasses /guilds entirely (handlers_auth.go redirects target=="posts"
//     straight to /guild/{id}/posts). Listing post-mod guilds here would
//     require a GetGuildCommandPermissions REST call per guild on a cold
//     override cache.
//   - For cached guilds, member lookup is cache-only — no GetMember REST
//     fallback per guild. Disgo populates the member cache from gateway
//     events; an admin who is active in their guild is essentially always
//     cached. Admins who aren't can still navigate directly via URL or the
//     slash command.
//   - For *stuck* guilds (Disgo received the READY stub but never the
//     GUILD_CREATE, or the guild was evicted by a GUILD_DELETE with
//     unavailable=true and never restored) we fall back to GetGuild +
//     GetMember. Stuck guilds aren't in client.Caches.Guilds() at all, so
//     the cache loop above misses them entirely. The fallback is bounded by
//     the size of UnreadyGuildIDs ∪ UnavailableGuildIDs, which is normally
//     empty.
//
// Net effect: /guilds does zero Discord REST calls in steady state, and at
// most one GetGuild + one GetMember per stuck guild ID otherwise.
func handleGuilds(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var guilds []pages.GuildData
		seen := make(map[snowflake.ID]struct{})
		appendGuild := func(id snowflake.ID, name string, iconPtr *string) {
			if _, dup := seen[id]; dup {
				return
			}
			seen[id] = struct{}{}
			var icon string
			if iconPtr != nil {
				icon = *iconPtr
			}
			guilds = append(guilds, pages.GuildData{
				ID:   id.String(),
				Name: name,
				Icon: icon,
			})
		}

		for guild := range client.Caches.Guilds() {
			isAdmin := guild.OwnerID == session.UserID
			if !isAdmin {
				member, ok := client.Caches.Member(guild.ID, session.UserID)
				if !ok {
					continue
				}
				isAdmin = client.Caches.MemberPermissions(member).Has(discord.PermissionAdministrator)
				if !isAdmin {
					continue
				}
			}
			appendGuild(guild.ID, guild.Name, guild.Icon)
		}

		stuckIDs := append(client.Caches.UnreadyGuildIDs(), client.Caches.UnavailableGuildIDs()...)
		for _, gid := range stuckIDs {
			if _, dup := seen[gid]; dup {
				continue
			}
			g, err := client.Rest.GetGuild(gid, false)
			if err != nil {
				// A 404 here means the bot no longer has access to the
				// guild (kicked / left / banned) but Disgo still has the
				// stale ID in its unready/unavailable set. That's an
				// expected, recoverable state — debug rather than warn so
				// it doesn't spam every /guilds hit. Anything else (rate
				// limit, 5xx, auth failure) is worth surfacing.
				if restStatusCode(err) == http.StatusNotFound {
					slog.Debug("guilds: GetGuild fallback 404", "guild_id", gid)
				} else {
					slog.Warn("guilds: GetGuild fallback failed", "guild_id", gid, "err", err)
				}
				continue
			}

			isAdmin := g.OwnerID == session.UserID
			if !isAdmin {
				m, err := client.Rest.GetMember(gid, session.UserID)
				if err != nil {
					// 404 here is expected when the dashboard user simply
					// isn't a member of this guild. Other statuses (rate
					// limit, 5xx, 403) indicate a real problem and should
					// be visible.
					if restStatusCode(err) == http.StatusNotFound {
						slog.Debug("guilds: GetMember fallback 404", "guild_id", gid, "user_id", session.UserID)
					} else {
						slog.Warn("guilds: GetMember fallback failed", "guild_id", gid, "user_id", session.UserID, "err", err)
					}
					continue
				}
				if !stuckGuildIsAdmin(g.Roles, m.RoleIDs, gid) {
					continue
				}
			}
			appendGuild(g.ID, g.Name, g.Icon)
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

// restStatusCode returns the HTTP status code from a disgo *rest.Error,
// or 0 if err isn't a *rest.Error or has no associated response. Lets the
// caller distinguish "expected 404" (e.g. user not in guild, bot kicked)
// from "real problem" (rate limit, 5xx, auth) without unwrapping inline.
func restStatusCode(err error) int {
	var rerr *rest.Error
	if errors.As(err, &rerr) && rerr.Response != nil {
		return rerr.Response.StatusCode
	}
	return 0
}

// stuckGuildIsAdmin reports whether the dashboard user holds the
// Administrator permission in a guild that isn't in the bot's local cache.
// It mirrors disgo's cache-side MemberPermissions but operates purely on
// the role slice from Rest.GetGuild and the member role-ID slice from
// Rest.GetMember, since stuck guilds have no cached roles or members to
// consult.
//
// The owner short-circuit is intentionally omitted — the caller already
// checks OwnerID against the session user before invoking this. The
// communication-disabled (timeout) degrade pass that the cache helper
// applies is also skipped: it only matters for non-Administrator perms,
// and Administrator overrides timeouts anyway.
func stuckGuildIsAdmin(roles []discord.Role, memberRoleIDs []snowflake.ID, guildID snowflake.ID) bool {
	rolesByID := make(map[snowflake.ID]discord.Role, len(roles))
	for _, r := range roles {
		rolesByID[r.ID] = r
	}
	// The @everyone role's ID equals the guild's ID. If it's missing from
	// the payload (shouldn't happen, but Discord has surprised us before)
	// the zero-value Role gives zero perms, which is the safe default.
	perms := rolesByID[guildID].Permissions
	if perms.Has(discord.PermissionAdministrator) {
		return true
	}
	for _, rid := range memberRoleIDs {
		r, ok := rolesByID[rid]
		if !ok {
			continue
		}
		perms = perms.Add(r.Permissions)
		if perms.Has(discord.PermissionAdministrator) {
			return true
		}
	}
	return false
}

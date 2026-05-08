package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"

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
//   - Member lookup is cache-only — no GetMember REST fallback per guild.
//     Disgo populates the member cache from gateway events; an admin who is
//     active in their guild is essentially always cached. Admins who aren't
//     can still navigate directly via URL or the slash command.
//
// Net effect: /guilds does zero Discord REST calls, regardless of how many
// guilds the bot is in.
func handleGuilds(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var guilds []pages.GuildData
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

			var icon string
			if guild.Icon != nil {
				icon = *guild.Icon
			}
			guilds = append(guilds, pages.GuildData{
				ID:   guild.ID.String(),
				Name: guild.Name,
				Icon: icon,
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

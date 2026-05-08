package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/disgoorg/disgo/bot"

	"github.com/NLLCommunity/heimdallr/interactions/post_dashboard"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

func handleGuilds(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var guilds []pages.GuildData
		for guild := range client.Caches.Guilds() {
			// Owner shortcut avoids any member fetch — they're admin by
			// definition and any member lookup for the iterated guild would
			// pay a REST round-trip on cache miss.
			isAdmin := guild.OwnerID == session.UserID
			isMod := false
			if !isAdmin {
				// Single member fetch reused for both checks. If the user
				// isn't a member of this guild at all, skip it: GetMember
				// would 404 and both checks would short-circuit anyway.
				member := guildMember(client, guild.ID, session.UserID)
				if member == nil {
					continue
				}
				isAdmin = isGuildAdminMember(client, guild, member)
				if !isAdmin {
					isMod = canUsePostDashboardForMember(client, guild, member, post_dashboard.CommandID(), post_dashboard.DefaultMemberPerm)
				}
			}
			if !isAdmin && !isMod {
				continue
			}

			var icon string
			if guild.Icon != nil {
				icon = *guild.Icon
			}
			guilds = append(guilds, pages.GuildData{
				ID:        guild.ID.String(),
				Name:      guild.Name,
				Icon:      icon,
				IsAdmin:   isAdmin,
				IsPostMod: isAdmin || isMod,
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

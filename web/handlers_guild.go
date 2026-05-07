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
			isAdmin := isGuildAdmin(client, guild, session.UserID)
			isMod := false
			if !isAdmin {
				isMod = canUsePostDashboard(client, guild, session.UserID, post_dashboard.CommandID(), post_dashboard.DefaultMemberPerm)
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

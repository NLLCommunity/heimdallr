package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/disgoorg/disgo/bot"

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
			if !isGuildAdmin(client, guild, session.UserID) {
				continue
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

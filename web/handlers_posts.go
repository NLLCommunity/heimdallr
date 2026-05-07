package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions/post_dashboard"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

// modGate runs the post-mod permission check using the captured slash command ID.
// Returns parsed guildID and true on success; writes the error response and
// returns false otherwise.
func modGate(w http.ResponseWriter, r *http.Request, client *bot.Client) (snowflake.ID, bool) {
	guildIDStr := r.PathValue("id")
	return checkGuildPostMod(w, r, client, guildIDStr, post_dashboard.CommandID(), post_dashboard.DefaultMemberPerm)
}

func handlePostsList(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}

		posts, err := model.ListPosts(guildID)
		if err != nil {
			slog.Error("ListPosts failed", "error", err, "guild_id", guildID)
			http.Error(w, "failed to load posts", http.StatusInternalServerError)
			return
		}

		guild, _ := client.Caches.Guild(guildID)
		nav := layouts.NavData{
			User:      session,
			GuildID:   guildID.String(),
			GuildName: guild.Name,
			IsAdmin:   isGuildAdmin(client, guild, session.UserID),
			IsPostMod: true,
		}

		renderSafe(w, r, pages.Posts(nav, pages.PostsData{
			GuildID: guildID.String(),
			Posts:   posts,
		}))
	}
}

func handlePostsCreate(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}

		post, err := model.CreatePost(guildID, "Untitled post", "[]", session.UserID)
		if err != nil {
			slog.Error("CreatePost failed", "error", err, "guild_id", guildID)
			http.Error(w, "failed to create post", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r,
			"/guild/"+guildID.String()+"/posts/"+strconv.FormatUint(uint64(post.ID), 10),
			http.StatusSeeOther,
		)
	}
}

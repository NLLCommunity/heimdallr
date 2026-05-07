package web

import (
	"errors"
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

func handlePostEditor(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}
		postID, err := strconv.ParseUint(r.PathValue("postID"), 10, 64)
		if err != nil {
			http.Error(w, "invalid post ID", http.StatusBadRequest)
			return
		}
		post, err := model.GetPost(guildID, uint(postID))
		if err != nil {
			http.Error(w, "post not found", http.StatusNotFound)
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
		renderSafe(w, r, pages.PostEditor(nav, pages.PostEditorData{
			GuildID:  guildID.String(),
			Post:     *post,
			Channels: guildChannels(client, guildID),
		}))
	}
}

func handlePostSave(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}
		postID, err := strconv.ParseUint(r.PathValue("postID"), 10, 64)
		if err != nil {
			http.Error(w, "invalid post ID", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		expectedVersion, err := strconv.ParseUint(r.FormValue("version"), 10, 64)
		if err != nil {
			http.Error(w, "invalid version", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		componentsJSON := r.FormValue("components_json")
		channelIDStr := r.FormValue("channel_id")
		var channelID snowflake.ID
		if channelIDStr != "" {
			channelID, err = snowflake.Parse(channelIDStr)
			if err != nil {
				http.Error(w, "invalid channel ID", http.StatusBadRequest)
				return
			}
		}

		_, err = model.UpdatePostFields(guildID, uint(postID), uint(expectedVersion), name, componentsJSON, channelID, session.UserID)
		switch {
		case errors.Is(err, model.ErrPostStaleVersion):
			http.Error(w, "this post was updated by someone else; reload and try again", http.StatusConflict)
			return
		case err != nil:
			slog.Error("UpdatePostFields failed", "error", err, "post_id", postID)
			http.Error(w, "failed to save post", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

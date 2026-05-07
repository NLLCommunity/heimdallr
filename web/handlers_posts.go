package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions/post_dashboard"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/posts"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
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

func handlePostPreview(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := modGate(w, r, client)
		if !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		raw := r.FormValue("components_json")
		var arr []any
		if err := json.Unmarshal([]byte(raw), &arr); err != nil {
			renderSafe(w, r, partials.PostSplitPreview(partials.PostSplitPreviewData{Error: "Invalid components JSON."}))
			return
		}
		chunks, err := posts.Plan(arr)
		if err != nil {
			renderSafe(w, r, partials.PostSplitPreview(partials.PostSplitPreviewData{Error: err.Error()}))
			return
		}
		strs := make([]string, len(chunks))
		for i, c := range chunks {
			b, _ := json.MarshalIndent(c, "", "  ")
			strs[i] = string(b)
		}
		renderSafe(w, r, partials.PostSplitPreview(partials.PostSplitPreviewData{Chunks: strs}))
	}
}

func handlePostPublish(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
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
		if post.ChannelID == 0 {
			http.Error(w, "select a channel before publishing", http.StatusBadRequest)
			return
		}

		var arr []any
		if err := json.Unmarshal([]byte(post.ComponentsJSON), &arr); err != nil {
			http.Error(w, "stored components are invalid; re-save the post", http.StatusUnprocessableEntity)
			return
		}
		chunks, err := posts.Plan(arr)
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot publish: %v", err), http.StatusBadRequest)
			return
		}

		existing, err := model.ListPostMessages(guildID, post.ID)
		if err != nil {
			slog.Error("ListPostMessages failed", "error", err, "post_id", post.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		existingMsgs := make([]posts.ExistingMessage, len(existing))
		for i, e := range existing {
			existingMsgs[i] = posts.ExistingMessage{ChannelID: e.ChannelID, MessageID: e.MessageID}
		}

		dc := posts.NewLiveDiscord(client, guildID)
		result, syncErr := posts.Sync(dc, posts.SyncPlan{NewChunks: chunks, ChannelID: post.ChannelID}, existingMsgs)

		// Persist created messages first — even if Sync returned an error mid-recreate,
		// the messages it managed to send are already on Discord and need to be tracked
		// in the DB so subsequent publishes don't orphan them or try to edit deleted
		// originals. ReplacePostMessages atomically swaps the set in a transaction.
		if result.RecreatedAll || (syncErr != nil && len(result.Created) > 0) {
			rows := make([]model.PostMessage, len(result.Created))
			for i, c := range result.Created {
				rows[i] = model.PostMessage{ChannelID: c.ChannelID, MessageID: c.MessageID}
			}
			if perr := model.ReplacePostMessages(post.ID, rows); perr != nil {
				slog.Error("ReplacePostMessages failed during partial-recreate persist", "error", perr, "post_id", post.ID)
			}
		} else if len(existing) == 0 && len(result.Created) > 0 {
			// First-publish success path (no recreate, no existing).
			rows := make([]model.PostMessage, len(result.Created))
			for i, c := range result.Created {
				rows[i] = model.PostMessage{ChannelID: c.ChannelID, MessageID: c.MessageID}
			}
			if perr := model.ReplacePostMessages(post.ID, rows); perr != nil {
				slog.Error("ReplacePostMessages failed on first publish", "error", perr, "post_id", post.ID)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}

		if syncErr != nil {
			slog.Error("post sync failed", "error", syncErr, "post_id", post.ID)
			http.Error(w, "publish failed: "+syncErr.Error(), http.StatusBadGateway)
			return
		}

		// N == M edit-in-place path: nothing to persist.
		// N < M edit-and-trim path: drop trailing rows by ID.
		if !result.RecreatedAll && result.DeletedCount > 0 {
			for i := result.KeptCount; i < len(existing); i++ {
				if perr := model.DeletePostMessage(existing[i].ID); perr != nil {
					slog.Warn("DeletePostMessage failed", "error", perr, "id", existing[i].ID)
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handlePostUnpublish(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
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
		existing, err := model.ListPostMessages(guildID, post.ID)
		if err != nil {
			slog.Error("ListPostMessages failed", "error", err, "post_id", post.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		dc := posts.NewLiveDiscord(client, guildID)
		for _, e := range existing {
			_ = dc.Delete(e.ChannelID, e.MessageID)
		}
		if err := model.ReplacePostMessages(post.ID, nil); err != nil {
			slog.Error("ReplacePostMessages(nil) failed", "error", err, "post_id", post.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handlePostDelete(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
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
		existing, _ := model.ListPostMessages(guildID, post.ID)
		dc := posts.NewLiveDiscord(client, guildID)
		for _, e := range existing {
			_ = dc.Delete(e.ChannelID, e.MessageID)
		}
		if err := model.DeletePost(guildID, post.ID); err != nil {
			slog.Error("DeletePost failed", "error", err, "post_id", post.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_ = session
		w.WriteHeader(http.StatusNoContent)
	}
}

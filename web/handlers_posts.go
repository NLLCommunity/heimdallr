package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"

	"github.com/NLLCommunity/heimdallr/interactions/post_dashboard"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/posts"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

// publishLocks serializes publish/unpublish operations per post. Without it,
// two moderators clicking Publish concurrently can both run posts.Sync against
// the same Discord state and leave orphaned messages or stale post_messages
// rows. TryLock + 409 surfaces contention to the user instead of silently
// racing.
//
// Entries are removed by handlePostDelete since no future request will target
// a deleted post; live posts retain a single entry for their lifetime.
//
// Single-process only: the dashboard runs as one bot, so a sync.Map is enough.
// If we ever scale to multiple replicas, this needs to move to a row lock or
// a distributed lock.
var publishLocks sync.Map // map[uint]*sync.Mutex

func acquirePublishLock(postID uint) (*sync.Mutex, bool) {
	m, _ := publishLocks.LoadOrStore(postID, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	return mu, mu.TryLock()
}

// modGate runs the post-mod permission check using the captured slash command ID.
// Returns parsed guildID and true on success; writes the error response and
// returns false otherwise.
func modGate(w http.ResponseWriter, r *http.Request, client *bot.Client) (snowflake.ID, bool) {
	guildIDStr := r.PathValue("id")
	return checkGuildPostMod(w, r, client, guildIDStr, post_dashboard.CommandID(), post_dashboard.DefaultMemberPerm)
}

// validatePostComponents parses the editor's components_json payload and runs
// it through the splitter so structural problems surface at save time instead
// of becoming "poisoned" rows that only blow up at preview/publish.
func validatePostComponents(componentsJSON string) error {
	var arr []any
	if err := json.Unmarshal([]byte(componentsJSON), &arr); err != nil {
		return fmt.Errorf("invalid components JSON: %w", err)
	}
	if _, err := posts.Plan(arr); err != nil {
		return err
	}
	return nil
}

// channelInGuild reports whether channelID resolves to a known message channel
// inside guildID. Used to keep cross-guild channel IDs out of the database and
// to re-validate at publish time in case the bot lost access (or the row was
// written before this check existed).
func channelInGuild(client *bot.Client, guildID, channelID snowflake.ID) bool {
	ch, ok := client.Caches.GuildMessageChannel(channelID)
	return ok && ch.GuildID() == guildID
}

func handlePostsList(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
			return
		}

		postEntries, err := model.ListPostsWithCounts(guildID)
		if err != nil {
			slog.Error("ListPostsWithCounts failed", "error", err, "guild_id", guildID)
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
			Posts:   postEntries,
		}))
	}
}

// handlePostsNew renders the editor for an unsaved post. Nothing is persisted
// until the user clicks Save — handlePostsCreate inserts the row using the
// submitted form data and the JS redirects to /posts/{id}.
func handlePostsNew(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildID, ok := modGate(w, r, client)
		if !ok {
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
			Post:     model.Post{ComponentsJSON: "[]"},
			Channels: guildChannels(client, guildID),
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
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		name := r.FormValue("name")
		if name == "" {
			name = "Untitled post"
		}
		componentsJSON := r.FormValue("components_json")
		if componentsJSON == "" {
			componentsJSON = "[]"
		}
		if err := validatePostComponents(componentsJSON); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		channelIDStr := r.FormValue("channel_id")
		var channelID snowflake.ID
		if channelIDStr != "" {
			id, err := snowflake.Parse(channelIDStr)
			if err != nil {
				http.Error(w, "invalid channel ID", http.StatusBadRequest)
				return
			}
			if !channelInGuild(client, guildID, id) {
				http.Error(w, "channel not found in this guild", http.StatusBadRequest)
				return
			}
			channelID = id
		}

		post, err := model.CreatePost(guildID, name, componentsJSON, channelID, session.UserID)
		if err != nil {
			slog.Error("CreatePost failed", "error", err, "guild_id", guildID)
			http.Error(w, "failed to create post", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]uint{
			"id":      post.ID,
			"version": post.Version,
		})
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
		if name == "" {
			// Mirror handlePostsCreate so a saved post can never end up with a
			// blank name in the list view.
			name = "Untitled post"
		}
		componentsJSON := r.FormValue("components_json")
		if err := validatePostComponents(componentsJSON); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		channelIDStr := r.FormValue("channel_id")
		var channelID snowflake.ID
		if channelIDStr != "" {
			channelID, err = snowflake.Parse(channelIDStr)
			if err != nil {
				http.Error(w, "invalid channel ID", http.StatusBadRequest)
				return
			}
			if !channelInGuild(client, guildID, channelID) {
				http.Error(w, "channel not found in this guild", http.StatusBadRequest)
				return
			}
		}

		updated, err := model.UpdatePostFields(guildID, uint(postID), uint(expectedVersion), name, componentsJSON, channelID, session.UserID)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			http.Error(w, "post not found", http.StatusNotFound)
			return
		case errors.Is(err, model.ErrPostStaleVersion):
			http.Error(w, "this post was updated by someone else; reload and try again", http.StatusConflict)
			return
		case err != nil:
			slog.Error("UpdatePostFields failed", "error", err, "post_id", postID)
			http.Error(w, "failed to save post", http.StatusInternalServerError)
			return
		}

		// Return the bumped version so the client can keep editing without
		// reloading. Without this, the next save would 409 with a stale
		// expectedVersion.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]uint{"version": updated.Version})
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
			b, err := json.MarshalIndent(c, "", "  ")
			if err != nil {
				renderSafe(w, r, partials.PostSplitPreview(partials.PostSplitPreviewData{Error: "Failed to render preview chunk: " + err.Error()}))
				return
			}
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
		mu, acquired := acquirePublishLock(uint(postID))
		if !acquired {
			http.Error(w, "another publish/unpublish is in progress for this post; try again in a moment", http.StatusConflict)
			return
		}
		defer mu.Unlock()
		post, err := model.GetPost(guildID, uint(postID))
		if err != nil {
			http.Error(w, "post not found", http.StatusNotFound)
			return
		}
		if post.ChannelID == 0 {
			http.Error(w, "select a channel before publishing", http.StatusBadRequest)
			return
		}
		// Re-validate at publish time. Save-time validation should already
		// have caught cross-guild IDs, but legacy rows or a bot that lost
		// access to the channel would otherwise have us send/edit/delete
		// outside this guild.
		if !channelInGuild(client, guildID, post.ChannelID) {
			http.Error(w, "channel not found in this guild; pick another channel and re-save", http.StatusBadRequest)
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

		// Log best-effort delete failures from Sync (e.g. messages already
		// removed manually or permissions changed) so an operator can spot
		// orphans. The publish itself can still succeed.
		for _, f := range result.DeleteFailures {
			slog.Warn("post sync: failed to delete Discord message during publish",
				"error", f.Err,
				"post_id", post.ID,
				"channel_id", f.ChannelID,
				"message_id", f.MessageID,
			)
		}

		// Persist created messages first — even if Sync returned an error mid-recreate,
		// the messages it managed to send are already on Discord and need to be tracked
		// in the DB so subsequent publishes don't orphan them or try to edit deleted
		// originals. ReplacePostMessages atomically swaps the set in a transaction.
		// Persist failures here are fatal: Discord-side changes have already happened,
		// and silently logging would leave the DB pointing at deleted messages so
		// future publishes can't reconcile.
		if result.RecreatedAll || (syncErr != nil && len(result.Created) > 0) {
			rows := make([]model.PostMessage, len(result.Created))
			for i, c := range result.Created {
				rows[i] = model.PostMessage{ChannelID: c.ChannelID, MessageID: c.MessageID}
			}
			if perr := model.ReplacePostMessages(post.ID, rows); perr != nil {
				slog.Error("ReplacePostMessages failed after Discord-side recreate; DB and Discord are now divergent (manual cleanup may be required)",
					"error", perr,
					"post_id", post.ID,
					"created", result.Created,
				)
				http.Error(w, "publish committed on Discord but failed to persist; reload and retry", http.StatusInternalServerError)
				return
			}
		} else if len(existing) == 0 && len(result.Created) > 0 {
			// First-publish success path (no recreate, no existing).
			rows := make([]model.PostMessage, len(result.Created))
			for i, c := range result.Created {
				rows[i] = model.PostMessage{ChannelID: c.ChannelID, MessageID: c.MessageID}
			}
			if perr := model.ReplacePostMessages(post.ID, rows); perr != nil {
				slog.Error("ReplacePostMessages failed on first publish; messages live on Discord but untracked in DB",
					"error", perr,
					"post_id", post.ID,
					"created", result.Created,
				)
				http.Error(w, "publish committed on Discord but failed to persist; reload and retry", http.StatusInternalServerError)
				return
			}
		}

		if syncErr != nil {
			slog.Error("post sync failed", "error", syncErr, "post_id", post.ID)
			http.Error(w, "publish failed: "+syncErr.Error(), http.StatusBadGateway)
			return
		}

		// N == M edit-in-place path: nothing to persist.
		// N < M edit-and-trim path: trailing messages were deleted on Discord;
		// reflect that in the DB by atomically replacing the row set with just
		// the kept prefix. Failure here is fatal — leaving stale rows would
		// make the next publish try to edit messages that no longer exist.
		if !result.RecreatedAll && result.DeletedCount > 0 {
			kept := make([]model.PostMessage, result.KeptCount)
			for i := 0; i < result.KeptCount; i++ {
				kept[i] = model.PostMessage{ChannelID: existing[i].ChannelID, MessageID: existing[i].MessageID}
			}
			if perr := model.ReplacePostMessages(post.ID, kept); perr != nil {
				slog.Error("ReplacePostMessages failed after trim; stale rows may point at deleted Discord messages",
					"error", perr,
					"post_id", post.ID,
					"kept_count", result.KeptCount,
					"deleted_count", result.DeletedCount,
				)
				http.Error(w, "publish committed on Discord but failed to persist; reload and retry", http.StatusInternalServerError)
				return
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
		mu, acquired := acquirePublishLock(uint(postID))
		if !acquired {
			http.Error(w, "another publish/unpublish is in progress for this post; try again in a moment", http.StatusConflict)
			return
		}
		defer mu.Unlock()
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
			if derr := dc.Delete(e.ChannelID, e.MessageID); derr != nil {
				slog.Warn("unpublish: failed to delete Discord message",
					"error", derr,
					"post_id", post.ID,
					"channel_id", e.ChannelID,
					"message_id", e.MessageID,
				)
			}
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
		mu, acquired := acquirePublishLock(uint(postID))
		if !acquired {
			http.Error(w, "another publish/unpublish is in progress for this post; try again in a moment", http.StatusConflict)
			return
		}
		defer mu.Unlock()
		post, err := model.GetPost(guildID, uint(postID))
		if err != nil {
			http.Error(w, "post not found", http.StatusNotFound)
			return
		}
		// Bail out before touching Discord if we can't read the message set —
		// proceeding would silently orphan messages while still removing the
		// post row, which is harder to recover from than a retryable 5xx.
		existing, err := model.ListPostMessages(guildID, post.ID)
		if err != nil {
			slog.Error("ListPostMessages failed", "error", err, "post_id", post.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		dc := posts.NewLiveDiscord(client, guildID)
		for _, e := range existing {
			if derr := dc.Delete(e.ChannelID, e.MessageID); derr != nil {
				slog.Warn("delete: failed to remove Discord message",
					"error", derr,
					"post_id", post.ID,
					"channel_id", e.ChannelID,
					"message_id", e.MessageID,
				)
			}
		}
		if err := model.DeletePost(guildID, post.ID); err != nil {
			slog.Error("DeletePost failed", "error", err, "post_id", post.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// No future requests can target this post, so the per-post mutex is
		// dead weight; drop the sync.Map entry to keep the lock table from
		// growing unboundedly across the bot's lifetime.
		publishLocks.Delete(uint(postID))
		w.WriteHeader(http.StatusNoContent)
	}
}

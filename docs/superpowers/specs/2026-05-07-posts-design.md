# Posts: persistent, multi-message bot content with a moderator dashboard

**Date:** 2026-05-07
**Status:** Design approved; pending implementation plan

## Overview

Today, Heimdallr's "message sandbox" lets admins compose V2-component messages and either send them one-shot or load and edit a single bot-authored Discord message. Authored content is not retained anywhere — once sent, the bot has no record of it.

This spec adds **Posts**: persistent, versioned content authored in the dashboard, kept in the database, and synced to one or more Discord messages. A post can span multiple Discord messages so mods can publish long-form content (announcements, rules, FAQs) without bumping into Discord's per-message limits. Posts have a separate moderator-accessible dashboard, distinct from the existing admin dashboard.

## Goals

- Mods can author, save, edit, publish, and unpublish content that lives in the bot's database.
- A single post can produce multiple Discord messages, automatically split at component boundaries to fit Discord's per-message limits.
- Updates to a post intelligently update the live Discord messages, preferring in-place edits over re-creation, while keeping all of a post's messages contiguous in the channel.
- Moderators have access to the post dashboard without inheriting admin privileges. Admins can access both dashboards.

## Non-goals

- No template variables (Mustache) in posts — they're for static long-form content, not parameterized join/leave messages.
- No audit log / version history — only the current and last-published state are kept.
- No soft-delete — delete means delete.
- No rich-text or markdown-block authoring above the V2-component layer; the editor is the same V2 component builder used in the sandbox.
- No multi-channel cross-posting from a single post.
- No collaborative editing or pessimistic locking — optimistic version checks only.

## Permission model

A new slash command anchors moderator access to Discord's native command-permission system:

```go
var PostDashboardCommand = discord.SlashCommandCreate{
    Name:                     "post-dashboard",
    Description:              "Open the post-management dashboard.",
    Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
    DefaultMemberPermissions: omit.NewPtr(discord.PermissionManageMessages),
}
```

Server admins configure who can invoke it via Server Settings → Integrations → Heimdallr → command permissions. The web dashboard reads those overrides and asks: "would Discord allow this user to invoke `/post-dashboard` in this guild?".

A new helper in `web/post_perms.go` (`canUsePostDashboard`) implements the check:

1. Admins (`PermissionAdministrator`) short-circuit to `true` without an API call.
2. Otherwise, fetch the `/post-dashboard` command's per-guild overrides via `Rest.GetApplicationCommandPermissions`. Cache the override list per-guild with a ~5 minute TTL, invalidated on guild settings save.
3. Resolve "can this user invoke `/post-dashboard`?" against the overrides + user's roles per Discord's documented permission-resolution rules ([discord.com/developers/docs/interactions/application-commands#permissions](https://discord.com/developers/docs/interactions/application-commands#permissions)). The dashboard is not channel-scoped, so channel-specific overrides effectively apply (treat the dashboard as if it lives in every channel — the strictest channel rule wins for any concrete channel, but since we have no channel context, apply guild-wide overrides only).
4. If no overrides match, evaluate `DefaultMemberPermissions` against the user's computed perms.

Implementation note: the exact resolution order has subtle cases (user override > role override > @everyone override; deny beats allow at the same scope; channel-specific layered on top). The implementation should mirror Discord's reference implementation closely and be covered by `post_perms_test.go`.

A new `checkGuildPostMod(w, r, client, guildID)` helper in `web/context.go` mirrors `checkGuildAdmin` but defers to `canUsePostDashboard`. The existing `checkGuildAdmin` is unchanged. Admins always pass `checkGuildPostMod`; moderators only pass `checkGuildPostMod`, never `checkGuildAdmin`.

`layouts.NavData` gains `IsAdmin` and `IsPostMod` flags. The menu shows "Settings" only when `IsAdmin`, "Posts" when `IsAdmin || IsPostMod`, both when admin. The session itself is unchanged — what differs is which routes accept it.

The slash command's interaction handler reuses the existing `/admin-dashboard` flow: generate a 5-minute one-shot login code, respond ephemerally with a clickable link to `/callback?code=…&target=posts`. The `/callback` POST flow already handles code-to-session exchange; `target` just controls where to redirect after auth (`admin` → `/guilds`, `posts` → the post dashboard for the calling guild).

## Data model

Two new GORM models live in `model/`:

```go
// Post is the editable, versioned source-of-truth for a piece of long-form
// bot content. ComponentsJSON holds the V2 component array (same shape as the
// sandbox produces).
type Post struct {
    ID             uint         `gorm:"primaryKey"`
    GuildID        snowflake.ID `gorm:"index;not null"`
    Name           string       `gorm:"not null"`              // mod-facing label
    ChannelID      snowflake.ID                                 // 0 until first publish
    ComponentsJSON string       `gorm:"type:text;not null"`
    Version        uint         `gorm:"not null;default:1"`    // optimistic concurrency token
    CreatedAt      time.Time
    UpdatedAt      time.Time
    UpdatedBy      snowflake.ID                                 // session UserID at last save
}

// PostMessage tracks the Discord messages currently owned by a post, in
// publication order. Position is dense (0..N-1) so the sync algorithm can
// index by it.
type PostMessage struct {
    ID        uint         `gorm:"primaryKey"`
    PostID    uint         `gorm:"index;not null"`
    Position  int          `gorm:"not null"` // 0-indexed
    ChannelID snowflake.ID `gorm:"not null"`
    MessageID snowflake.ID `gorm:"not null"`
    CreatedAt time.Time
}
```

A post is **draft** if and only if it has zero `PostMessage` rows. There is no draft/published flag; the presence of `PostMessage` rows is the source of truth.

**Optimistic locking:** every save form submits the `Version` it loaded. The handler runs `UPDATE posts SET … , version = version + 1 WHERE id = ? AND version = ?`. Zero rows affected → 409 Conflict; the response carries the latest record so the client can show "someone else updated this post" and offer to reload.

**Re-targeting:** changing `ChannelID` is allowed. On the next publish, if any existing `PostMessage` rows live in a different channel, the publish flow deletes them (and the messages on Discord) before publishing fresh in the new channel.

## Splitting algorithm

Posts are V2-only — every Discord message a post produces is sent with `MessageFlagIsComponentsV2` set, both on first publish and on edits. A post's `ComponentsJSON` is an ordered array of top-level V2 components (the same shape the existing message-builder Alpine code produces). On publish, the splitter packs them into Discord messages:

```
plan(components) -> [][]component
  current := []
  out := []
  for comp in components:
    if fits(current + [comp]):
      current = current + [comp]
    else:
      if current is empty:
        ERROR: single component too large to fit a message
      out = out + [current]
      current = [comp]
  if current is non-empty: out = out + [current]
  return out
```

`fits()` enforces Discord's per-message caps:

- ≤ 40 components total, counting nested children of containers/sections.
- ≤ 4000 characters summed across all `text_display.content` in the message.
- ≤ 10 attachments / media-gallery items.

These caps live as constants in a new `web/posts/splitter.go` so they're easy to tune as Discord's limits evolve. `fits()` and `plan()` are pure functions and unit-testable in isolation.

A single component that exceeds a per-component cap (e.g. a `text_display` over 4000 chars) is rejected with "this component is too large; split it manually". The splitter never breaks inside a component.

Validation runs both at save time (so the user gets early feedback) and again at publish time (defense in depth: the saved JSON could in principle reach the publish path through other routes).

## Sync (publish/update) algorithm

Publish requires `post.ChannelID != 0`. A publish call on a post with no channel set returns 400 with "select a channel before publishing".

```
sync(post):
    plan := split(post.components)            // [][]component, length N
    existing := postMessages(post.id)         // ordered, length M

    if any(existing[i].ChannelID != post.ChannelID for i in 0..M-1):
        // Re-targeting: drop everything, fall through as first publish.
        for msg in existing: best-effort delete on Discord
        delete all PostMessage rows for post
        existing = []
        M = 0

    if M == 0:                                // first publish
        for chunk in plan:
            send chunk in post.ChannelID
            insert PostMessage row
        return

    if N > M:                                 // need more messages than we have
        // Can't insert in the middle — Discord may have other messages
        // between ours by now. Recreate at the bottom of the channel
        // so all post messages stay contiguous.
        for msg in existing: best-effort delete on Discord
        delete PostMessage rows
        for chunk in plan:
            send chunk; insert PostMessage row
        return

    // N <= M: edit in place, drop trailing surplus
    for i in 0..N-1:
        edit existing[i] with plan[i]
    for i in N..M-1:
        best-effort delete existing[i] on Discord
        delete PostMessage row
```

**Failure handling:**

- Discord delete failures (message manually removed, permissions changed) are logged and skipped — the DB row is dropped either way, since the goal is alignment with the new state.
- Discord edit failures abort the sync and surface the error to the mod. The DB stays at the old `Version`; the mod can retry.
- Discord send failures during a recreate (e.g. message 3 of 5 fails after 1 and 2 succeeded) abort the sync and leave whatever was created in place, recorded in `PostMessage`. The next publish attempt sees them as the "existing" set and proceeds normally.

**No locking:** sync runs inside the publish handler. The optimistic version check at the top of publish prevents two mods from overlapping syncs; the second one gets a 409.

## Web surface

New routes under the existing middleware chain (auth → body limit → rate limit → CORS):

```
GET  /guild/{id}/posts                    list posts (mod-or-admin)
GET  /guild/{id}/posts/new                editor page, blank post
POST /guild/{id}/posts                    create (returns redirect to editor)
GET  /guild/{id}/posts/{postID}           editor page for existing post
POST /guild/{id}/posts/{postID}           save (DB only, version-checked)
POST /guild/{id}/posts/{postID}/publish   publish/update on Discord (sync)
POST /guild/{id}/posts/{postID}/unpublish delete from Discord; record stays as draft
POST /guild/{id}/posts/{postID}/delete    hard-delete record + best-effort Discord cleanup
POST /guild/{id}/posts/{postID}/preview   render split preview (components → N message previews)
```

All gated by `checkGuildPostMod`. Admins automatically pass.

The editor page reuses the templ `MessageEditor` partial that the sandbox already uses. A new Alpine factory `postEditor()` in `static/js/post-editor.js` provides the editor's state — it has its own shape (no `loadLink`/`loadedMessageId`/etc. since posts don't load arbitrary messages), but its `components` array, `addComponent`, `removeComponent`, `serialize`, etc. are the same shape `MessageEditor` references, so the partial works in both contexts. The `serializeComponent` / `deserializeComponent` helpers stay in `message-builder.js` and are shared.

The split-preview is a partial that re-renders on every editor change via HTMX: post the current `components_json` to `/preview`, the server runs `split()`, and returns the preview HTML showing "Message 1 of 3", "Message 2 of 3", etc. The splitting algorithm stays Go-side as a single source of truth; the JS does not reimplement it.

New files:

- `web/handlers_posts.go` — HTTP handlers.
- `web/post_perms.go` — `canUsePostDashboard` + permission cache.
- `web/templates/pages/posts.templ` — list page.
- `web/templates/pages/post_editor.templ` — editor page.
- `web/templates/partials/post_split_preview.templ` — preview partial.
- `web/posts/splitter.go` — pure-Go splitting algorithm + tests.
- `interactions/post_dashboard/post_dashboard.go` — slash command.
- `model/post.go` — GORM models + helpers.
- `web/static/js/post-editor.js` — Alpine factory for the post editor (separate from `messageBuilder` to avoid loading sandbox-only state).

`web/server.go` wires the new routes, registers a `postsLimiter` (per-user, modeled on `sandboxLimiter`) for publish/unpublish/delete (not save — DB-only).

`main.go` registers the new slash command via the existing `interactions.ApplicationCommandRegisterFunc` mechanism.

Migration: the new GORM models join the existing `AutoMigrate` call in `model/`'s init.

## Testing

- `splitter_test.go` covers: empty input; single small component; chain that exactly fills one message; chain that overflows by one component; oversized single component (error); component-count cap; character cap; media-cap.
- `sync_test.go` covers each branch of the sync algorithm (first publish, N == M, N < M, N > M, re-target). Discord REST is faked via an interface for these tests; integration with real Discord is out of scope for the spec.
- `post_perms_test.go` covers the permission walk: admin short-circuit; no overrides; channel override true; role override false; user override beats role; cache hit/miss.
- `handlers_posts_test.go` covers the version-conflict (409) path, basic CRUD round-trips, and that mod-only routes reject non-mod sessions while admin routes still work for admins.

## Out of scope (not in this work)

- Scheduling / future-dated publishes.
- Reactions or analytics on posted messages.
- Templating (Mustache) in posts.
- Slash-command subcommands like `/post-dashboard list` / `… create` — the dashboard is the only entry point.
- Per-post audit/history.
- Search / tagging across posts beyond the list view.

package audit

import (
	"encoding/json"
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"
)

// ResolveRoleName looks up the role's name from the disgo cache, returning
// the snowflake ID stringified on miss. Used by listeners building Details
// payloads so the audit log viewer can render readable role names without
// re-fetching at view time.
func ResolveRoleName(client *bot.Client, guildID, roleID snowflake.ID) string {
	if role, ok := client.Caches.Role(guildID, roleID); ok {
		return role.Name
	}
	return fmt.Sprintf("role:%d", roleID)
}

// ResolveChannelName looks up the channel's name from the disgo cache.
func ResolveChannelName(client *bot.Client, channelID snowflake.ID) string {
	if ch, ok := client.Caches.Channel(channelID); ok {
		return ch.Name()
	}
	return fmt.Sprintf("channel:%d", channelID)
}

// ResolveMemberUsername looks up a user's stable Discord username (the
// @handle) from the disgo member cache. Returns "" on miss so callers can
// chain to other resolution sources (e.g. usernames captured in audit log
// Details payloads at write time).
//
// Audit listeners must stay fast and the snowflake ID is always recorded
// anyway, so missing names are not load-bearing.
func ResolveMemberUsername(client *bot.Client, guildID, userID snowflake.ID) string {
	if member, ok := client.Caches.Member(guildID, userID); ok {
		return member.User.Username
	}
	return ""
}

// ResolveMemberUsernameOrFetch is ResolveMemberUsername with a REST
// fallback that fetches the member when the cache misses, then warms
// the cache so subsequent lookups (e.g. during the same audit log
// burst) hit the in-memory path.
//
// Slower than the cache-only call — adds a network round-trip on miss
// — so use this from audit-log enrichment paths where the trade-off
// (latency vs. a missing name in the viewer) favours fetching, not
// from hot listener paths that fire per-message.
//
// Returns "" only when both the cache and the REST call fail (e.g. the
// user is no longer a guild member and was never cached).
func ResolveMemberUsernameOrFetch(client *bot.Client, guildID, userID snowflake.ID) string {
	if name := ResolveMemberUsername(client, guildID, userID); name != "" {
		return name
	}
	member, err := client.Rest.GetMember(guildID, userID)
	if err != nil || member == nil {
		return ""
	}
	// Prime the cache so the next call in this same audit burst hits
	// the in-memory path. No-op if the member cache flag is disabled.
	if mc := client.Caches.MemberCache(); mc != nil {
		mc.Put(guildID, userID, *member)
	}
	return member.User.Username
}

// FormatActor returns a human-readable label for the actor side of an
// audit log row. Used by the web viewer to show "@modUser" instead of a
// raw snowflake.
//
// detailsJSON is consulted as a fallback when the member cache misses —
// listeners record actor_username (and target username) at write time so
// we can render names for users no longer in the guild without a REST
// round-trip.
//
// Returns "—" when the actor is missing or unknown — the alternative is
// a snowflake ID, which is not actionable for the moderator browsing the
// log.
func FormatActor(client *bot.Client, guildID snowflake.ID, kind ActorKind, id *snowflake.ID, detailsJSON string) string {
	if id == nil {
		switch kind {
		case ActorBot:
			return "Bot"
		case ActorSystem:
			return "System"
		}
		return "—"
	}
	name := resolveUsername(client, guildID, *id, detailsJSON, "actor_username")
	if name == "" {
		name = id.String()
	}
	switch kind {
	case ActorBot:
		return "@" + name + " (bot)"
	case ActorSystem:
		return "System"
	}
	// ActorUser, ActorUnknown — both render the same way; ActorUnknown
	// with a known ID happens when a self-action lacked native audit log
	// attribution.
	return "@" + name
}

// FormatTarget returns a human-readable label for the target side of an
// audit log row.
//
// For message events the target ID is the now-deleted message and isn't
// meaningful to a viewer; the channel where the message lived is more
// informative, so we pull channel_id out of the JSON details payload and
// render the channel name instead.
//
// detailsJSON may be empty — the function falls back to ID-only labels in
// that case.
func FormatTarget(
	client *bot.Client,
	guildID snowflake.ID,
	kind TargetKind,
	id *snowflake.ID,
	detailsJSON string,
) string {
	switch kind {
	case TargetGuild, TargetNone:
		// Showing the guild ID as the target is noise — every entry on
		// this page is already scoped to the current guild.
		return "—"

	case TargetMessage:
		// The message itself is gone; surface the channel for context.
		if chID := channelIDFromDetails(detailsJSON); chID != 0 {
			return "#" + ResolveChannelName(client, chID)
		}
		if id != nil {
			return fmt.Sprintf("message:%d", *id)
		}
		return "—"

	case TargetChannel:
		if id != nil {
			return "#" + ResolveChannelName(client, *id)
		}
		return "—"

	case TargetRole:
		if id != nil {
			return "@" + ResolveRoleName(client, guildID, *id)
		}
		return "—"

	case TargetUser:
		if id != nil {
			name := resolveUsername(client, guildID, *id, detailsJSON, "target_username")
			if name == "" {
				name = id.String()
			}
			return "@" + name
		}
		return "—"
	}
	if id != nil {
		return id.String()
	}
	return "—"
}

// resolveUsername resolves a user ID to a username via a layered lookup:
// the disgo member cache (current name in the guild), then the named
// field from the audit log's stored Details JSON. Returns "" if neither
// source has a name.
//
// detailsField names the JSON key to consult — by convention listeners
// record the target as "target_username" (paired with target_id) and the
// actor as "actor_username" (paired with actor_id). The function does
// NOT fall back across fields: rendering the actor as the target's
// username (or vice versa) would silently produce wrong attribution. For
// events where actor == target, the listener stores both fields.
func resolveUsername(client *bot.Client, guildID, userID snowflake.ID, detailsJSON, detailsField string) string {
	if name := ResolveMemberUsername(client, guildID, userID); name != "" {
		return name
	}
	return usernameFromDetails(detailsJSON, detailsField)
}

// usernameFromDetails extracts the named field from a stored Details JSON
// payload. Returns "" on any parse failure or missing key.
func usernameFromDetails(detailsJSON, field string) string {
	if detailsJSON == "" {
		return ""
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		return ""
	}
	if v, ok := d[field].(string); ok {
		return v
	}
	return ""
}

// channelIDFromDetails extracts the "channel_id" field from a stored
// audit-log Details JSON payload. Listeners write it as a string (via
// snowflake.ID.String) so the JSON shape is always {"channel_id": "..."}.
//
// Returns 0 on any parse miss; callers treat that as "no channel context".
func channelIDFromDetails(detailsJSON string) snowflake.ID {
	if detailsJSON == "" {
		return 0
	}
	var d struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		return 0
	}
	if d.ChannelID == "" {
		return 0
	}
	id, err := snowflake.Parse(d.ChannelID)
	if err != nil {
		return 0
	}
	return id
}

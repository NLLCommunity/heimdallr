package audit

import (
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
func ResolveMemberUsername(client *bot.Client, guildID, userID snowflake.ID) (username string, ok bool) {
	if member, ok := client.Caches.Member(guildID, userID); ok {
		return member.User.Username, true
	}
	return "", false
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
	if name, ok := ResolveMemberUsername(client, guildID, userID); ok {
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

// ResolveUserUsernameOrFetch resolves a user's @handle via the disgo
// member cache (in case they're still cached as a guild member),
// falling back to a REST GetUser call that works regardless of guild
// membership.
//
// Use this for kick / ban target resolution: by the time the native
// audit log entry fires, Discord has already removed the user from the
// guild, so Rest.GetMember 404s and ResolveMemberUsernameOrFetch
// returns "". GetUser still works because the user account itself
// exists independently of guild membership.
//
// Returns "" only when both lookups fail (deleted Discord account, or
// REST error).
func ResolveUserUsernameOrFetch(client *bot.Client, guildID, userID snowflake.ID) string {
	if name, ok := ResolveMemberUsername(client, guildID, userID); ok {
		return name
	}
	user, err := client.Rest.GetUser(userID)
	if err != nil || user == nil {
		return ""
	}
	return user.Username
}

// FormatActor returns a human-readable label for the actor side of an
// audit log row. Used by the web viewer to show "@modUser" instead of a
// raw snowflake.
//
// details is the already-decoded entry.Details map (or nil). The caller
// decodes once per row; passing the decoded map here lets a 50-row page
// avoid 100+ redundant JSON unmarshals when also calling FormatTarget,
// summariseDetail, etc. on the same payload.
//
// The cached-name lookup uses the disgo member cache first; details is
// consulted as a fallback when the cache misses — listeners record
// actor_username at write time so we can render names for users no longer
// in the guild without a REST round-trip.
//
// Returns "—" when the actor is missing or unknown.
func FormatActor(client *bot.Client, guildID snowflake.ID, kind ActorKind, id *snowflake.ID, details map[string]any) string {
	if id == nil {
		switch kind {
		case ActorBot:
			return "Bot"
		case ActorSystem:
			return "System"
		}
		return "—"
	}
	name := resolveUsername(client, guildID, *id, details, "actor_username")
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
// informative, so we pull channel_id out of the details payload and
// render the channel name instead.
//
// details is the already-decoded entry.Details map (or nil); see
// FormatActor for the rationale.
func FormatTarget(
	client *bot.Client,
	guildID snowflake.ID,
	kind TargetKind,
	id *snowflake.ID,
	details map[string]any,
) string {
	switch kind {
	case TargetGuild, TargetNone:
		// Showing the guild ID as the target is noise — every entry on
		// this page is already scoped to the current guild.
		return "—"

	case TargetMessage:
		// The message itself is gone; surface the channel for context.
		if chID := channelIDFromDetails(details); chID != 0 {
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
			name := resolveUsername(client, guildID, *id, details, "target_username")
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
// field from the already-decoded details map. Returns "" if neither
// source has a name.
//
// detailsField names the map key to consult — by convention listeners
// record the target as "target_username" (paired with target_id) and the
// actor as "actor_username" (paired with actor_id). The function does
// NOT fall back across fields: rendering the actor as the target's
// username (or vice versa) would silently produce wrong attribution. For
// events where actor == target, the listener stores both fields.
func resolveUsername(client *bot.Client, guildID, userID snowflake.ID, details map[string]any, detailsField string) string {
	if name, ok := ResolveMemberUsername(client, guildID, userID); ok {
		return name
	}
	if v, ok := details[detailsField].(string); ok {
		return v
	}
	return ""
}

// channelIDFromDetails extracts the "channel_id" field from an
// already-decoded details map. Listeners write it as a string (via
// snowflake.ID.String) so the shape is always {"channel_id": "..."}.
//
// Returns 0 on any parse miss; callers treat that as "no channel context".
func channelIDFromDetails(details map[string]any) snowflake.ID {
	s, ok := details["channel_id"].(string)
	if !ok || s == "" {
		return 0
	}
	id, err := snowflake.Parse(s)
	if err != nil {
		return 0
	}
	return id
}

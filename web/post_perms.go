package web

import (
	"slices"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
)

// guildMember returns the user's member record for the guild, hitting the
// member cache first and falling back to a REST GetMember on miss. Returns
// nil when the user isn't a member of the guild — REST 404 and transient
// errors are indistinguishable here, but both mean "no access" for the
// permission checks that call this.
//
// One REST call per non-cached lookup is acceptable because callers only
// reach this path after intersecting with the bot's guild list (so we
// never probe random guilds the user has no reason to be in) and the
// posts-role check is only performed for non-admin users in guilds that
// explicitly opted into post-mod access.
func guildMember(client *bot.Client, guildID, userID snowflake.ID) *discord.Member {
	if m, ok := client.Caches.Member(guildID, userID); ok {
		return &m
	}
	m, err := client.Rest.GetMember(guildID, userID)
	if err != nil {
		return nil
	}
	return m
}

// isGuildAdminMember tests admin against an already-fetched member. The
// owner shortcut still compares Member.User.ID rather than a raw userID so
// callers don't need to plumb both through.
func isGuildAdminMember(client *bot.Client, guild discord.Guild, member *discord.Member) bool {
	if member == nil {
		return false
	}
	if guild.OwnerID == member.User.ID {
		return true
	}
	return client.Caches.MemberPermissions(*member).Has(discord.PermissionAdministrator)
}

// hasPostsModRole reports whether the member holds the guild's configured
// posts-mod role. Returns false when no role is configured, when the member
// is nil, or when the member lacks the role. Callers should fold this
// together with the admin check via canManagePostsForMember rather than
// using it standalone — admins must always pass the access check
// regardless of whether the role is configured or held.
func hasPostsModRole(settings *model.GuildSettings, member *discord.Member) bool {
	if settings == nil || settings.PostsModRoleID == 0 || member == nil {
		return false
	}
	// A timed-out (communication-disabled) member must not keep posts
	// access: Discord strips their guild permissions for the duration,
	// and the dashboard gate has to match. The old permission path went
	// through Caches.MemberPermissions, which disgo degrades to
	// ViewChannel|ReadMessageHistory during a timeout; a bare RoleIDs
	// lookup would silently bypass that. Admins are unaffected (Discord
	// does not let members with Administrator be timed out), mirroring
	// how MemberPermissions degrades non-admins only.
	if member.CommunicationDisabledUntil != nil && member.CommunicationDisabledUntil.After(time.Now()) {
		return false
	}
	return slices.Contains(member.RoleIDs, settings.PostsModRoleID)
}

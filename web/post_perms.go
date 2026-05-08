package web

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// resolveCommandPermission applies Discord's documented precedence rules for
// application-command permission overrides:
//
//  1. If a user-specific override matches, it wins outright (the override is
//     a single boolean, allow or deny).
//  2. Otherwise, walk role overrides for every role the user has plus the
//     @everyone role (which has the same ID as the guild). Any allowing role
//     grants access; only deny if no matching role allows.
//  3. If no overrides match, fall back to the command's default permission
//     (the caller computes whether DefaultMemberPermissions admits the user
//     and passes it as defaultAllow).
//
// userRoles excludes @everyone; the @everyone role ID equals the guildID.
func resolveCommandPermission(
	overrides []discord.ApplicationCommandPermission,
	userID snowflake.ID,
	userRoles []snowflake.ID,
	guildID snowflake.ID,
	defaultAllow bool,
) bool {
	for _, o := range overrides {
		if u, ok := o.(discord.ApplicationCommandPermissionUser); ok && u.UserID == userID {
			return u.Permission
		}
	}

	matchingRoles := make(map[snowflake.ID]bool)
	for _, rid := range append([]snowflake.ID{guildID}, userRoles...) {
		for _, o := range overrides {
			if r, ok := o.(discord.ApplicationCommandPermissionRole); ok && r.RoleID == rid {
				matchingRoles[rid] = r.Permission
			}
		}
	}
	if len(matchingRoles) > 0 {
		for _, allow := range matchingRoles {
			if allow {
				return true
			}
		}
		return false
	}

	return defaultAllow
}

type cachedOverrides struct {
	overrides []discord.ApplicationCommandPermission
	fetchedAt time.Time
}

// commandOverrideCache memoizes per-guild Discord command-permission overrides
// for a single command, with a TTL. The cache is stampede-tolerant (two
// concurrent callers may both fetch, but the result remains correct).
type commandOverrideCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[snowflake.ID]cachedOverrides
}

func newCommandOverrideCache(ttl time.Duration) *commandOverrideCache {
	return &commandOverrideCache{
		ttl: ttl,
		m:   make(map[snowflake.ID]cachedOverrides),
	}
}

// get returns cached overrides, calling fetch only on a miss or stale entry.
// fetch is invoked outside the mutex so it can do I/O without blocking other
// guilds' cache reads.
func (c *commandOverrideCache) get(guildID snowflake.ID, fetch func() ([]discord.ApplicationCommandPermission, error)) ([]discord.ApplicationCommandPermission, error) {
	c.mu.Lock()
	entry, ok := c.m[guildID]
	c.mu.Unlock()
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return entry.overrides, nil
	}
	overrides, err := fetch()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.m[guildID] = cachedOverrides{overrides: overrides, fetchedAt: time.Now()}
	c.mu.Unlock()
	return overrides, nil
}

func (c *commandOverrideCache) invalidate(guildID snowflake.ID) {
	c.mu.Lock()
	delete(c.m, guildID)
	c.mu.Unlock()
}

// postDashboardOverrideCache is the package-global cache for the
// /post-dashboard command's per-guild overrides. TTL-only invalidation
// (5 minutes): the bot has no UI to mutate Discord command-permission
// overrides, so changes made via Discord's UI surface here only after
// the TTL expires. The exposed invalidate hook exists for future
// settings paths that might cause overrides to drift.
var postDashboardOverrideCache = newCommandOverrideCache(5 * time.Minute)

// guildMember returns the user's member record for the guild, hitting the
// cache first and falling back to GetMember on miss. Returns nil when the
// user isn't a member of the guild — REST 404s and transient errors are
// indistinguishable here, but both mean "no access" for our purposes.
//
// Callers that need both an admin check and a post-mod check (handleDashboard,
// checkGuildPostMod) fetch the member once and pass it into isGuildAdminMember
// + canUsePostDashboardForMember, avoiding a second REST round-trip per guild
// on cache miss.
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

// canUsePostDashboardForMember resolves /post-dashboard overrides against an
// already-fetched member. Mirrors Discord's permission resolution: per-guild
// overrides on top of the command's default permission. Admin/owner
// short-circuits live at the call site so the member fetch is shared with
// the admin check (see checkGuildPostMod, handleDashboard).
//
// On any Discord-side error fetching overrides, returns false (fail-closed) —
// better to deny mod-only access on a transient error than to expose the
// moderator dashboard to someone whose permissions can't be confirmed.
func canUsePostDashboardForMember(client *bot.Client, guild discord.Guild, member *discord.Member, postDashboardCommandID snowflake.ID, defaultMemberPerm discord.Permissions) bool {
	if member == nil {
		return false
	}
	// If the command hasn't been registered yet (commandID == 0), nobody
	// non-admin can use it.
	if postDashboardCommandID == 0 {
		return false
	}

	overrides, err := postDashboardOverrideCache.get(guild.ID, func() ([]discord.ApplicationCommandPermission, error) {
		perms, err := client.Rest.GetGuildCommandPermissions(client.ApplicationID, guild.ID, postDashboardCommandID)
		if err != nil {
			var rerr *rest.Error
			if errors.As(err, &rerr) && rerr.Response != nil && rerr.Response.StatusCode == http.StatusNotFound {
				// No overrides configured for this guild — that's the common case.
				// Cache an empty list so we fall through to the default permission
				// without re-hitting Discord on every request.
				return []discord.ApplicationCommandPermission{}, nil
			}
			return nil, err
		}
		return perms.Permissions, nil
	})
	if err != nil {
		return false
	}

	memberPerms := client.Caches.MemberPermissions(*member)
	defaultAllow := memberPerms.Has(defaultMemberPerm)

	return resolveCommandPermission(overrides, member.User.ID, member.RoleIDs, guild.ID, defaultAllow)
}

package web

import (
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
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
// /post-dashboard command's per-guild overrides. TTL is 5 minutes;
// settings saves should call invalidate explicitly.
var postDashboardOverrideCache = newCommandOverrideCache(5 * time.Minute) //nolint:unused

// canUsePostDashboard returns true if the user is allowed to invoke the
// /post-dashboard command in the guild — admins always pass. It mirrors
// Discord's permission resolution: per-guild overrides on top of the
// command's default permission, with admin short-circuit.
//
// On any Discord-side error fetching overrides, the function returns false
// (fail-closed) — better to deny mod-only access on a transient error than
// expose the moderator dashboard to someone whose permissions can't be
// confirmed.
func canUsePostDashboard(client *bot.Client, guild discord.Guild, userID, postDashboardCommandID snowflake.ID, defaultMemberPerm discord.Permissions) bool { //nolint:unused
	if isGuildAdmin(client, guild, userID) {
		return true
	}

	// If the command hasn't been registered yet (commandID == 0), nobody
	// non-admin can use it.
	if postDashboardCommandID == 0 {
		return false
	}

	member, ok := client.Caches.Member(guild.ID, userID)
	if !ok {
		m, err := client.Rest.GetMember(guild.ID, userID)
		if err != nil {
			return false
		}
		member = *m
	}

	overrides, err := postDashboardOverrideCache.get(guild.ID, func() ([]discord.ApplicationCommandPermission, error) {
		perms, err := client.Rest.GetGuildCommandPermissions(client.ApplicationID, guild.ID, postDashboardCommandID)
		if err != nil {
			return nil, err
		}
		return perms.Permissions, nil
	})
	if err != nil {
		return false
	}

	memberPerms := client.Caches.MemberPermissions(member)
	defaultAllow := memberPerms.Has(defaultMemberPerm)

	return resolveCommandPermission(overrides, userID, member.RoleIDs, guild.ID, defaultAllow)
}

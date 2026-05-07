package web

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// resolveCommandPermission applies Discord's documented precedence rules for
// application-command permission overrides:
//
//  1. If a user-specific override matches, it wins outright (the override is
//     a single boolean, allow or deny).
//  2. Otherwise, walk role overrides for every role the user has plus the
//     @everyone role (which has the same ID as the guild). Deny beats allow
//     at the same scope: any matching deny → deny; otherwise any matching
//     allow → allow.
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
	// Pass 1: user override wins outright if present.
	for _, o := range overrides {
		if u, ok := o.(discord.ApplicationCommandPermissionUser); ok && u.UserID == userID {
			return u.Permission
		}
	}

	// Pass 2: collect matching role overrides (user's roles + @everyone).
	matchingRoles := make(map[snowflake.ID]bool)
	for _, rid := range append([]snowflake.ID{guildID}, userRoles...) {
		for _, o := range overrides {
			if r, ok := o.(discord.ApplicationCommandPermissionRole); ok && r.RoleID == rid {
				matchingRoles[rid] = r.Permission
			}
		}
	}
	if len(matchingRoles) > 0 {
		// Any allowing role wins; only deny if no role allows.
		for _, allow := range matchingRoles {
			if allow {
				return true
			}
		}
		return false
	}

	return defaultAllow
}

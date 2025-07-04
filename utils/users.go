package utils

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func HasRole(member discord.Member, roleID snowflake.ID) bool {
	for _, role := range member.RoleIDs {
		if role == roleID {
			return true
		}
	}

	return false
}

func HasRolesAll(member discord.Member, roleIDs ...snowflake.ID) bool {
	if len(roleIDs) == 0 {
		return true // Empty set means all conditions are satisfied.
	}

	memberRoles := make(map[snowflake.ID]bool)
	for _, role := range member.RoleIDs {
		memberRoles[role] = true
	}

	for _, requiredRole := range roleIDs {
		if !memberRoles[requiredRole] {
			return false
		}
	}

	return true
}

func HasRolesAny(member discord.Member, roleIDs ...snowflake.ID) bool {
	for _, role := range member.RoleIDs {
		for _, roleID := range roleIDs {
			if role == roleID {
				return true
			}
		}
	}

	return false
}

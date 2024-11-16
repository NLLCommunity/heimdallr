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
	hasRole := make(map[snowflake.ID]bool)
	for _, role := range member.RoleIDs {
		hasRole[role] = false
	}
	for _, role := range member.RoleIDs {
		for _, roleID := range roleIDs {
			if role == roleID {
				hasRole[role] = true
			}
		}
	}

	for _, hasRole := range hasRole {
		if !hasRole {
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

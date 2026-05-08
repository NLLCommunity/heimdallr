package web

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
)

const stuckGuildID snowflake.ID = 99

func roleWith(id snowflake.ID, perms discord.Permissions) discord.Role {
	return discord.Role{ID: id, Permissions: perms}
}

func TestStuckGuildIsAdmin_EveryoneHasAdmin(t *testing.T) {
	roles := []discord.Role{
		roleWith(stuckGuildID, discord.PermissionAdministrator),
	}
	assert.True(t, stuckGuildIsAdmin(roles, nil, stuckGuildID))
}

func TestStuckGuildIsAdmin_MemberRoleHasAdmin(t *testing.T) {
	roles := []discord.Role{
		roleWith(stuckGuildID, 0),
		roleWith(100, discord.PermissionSendMessages),
		roleWith(200, discord.PermissionAdministrator),
	}
	assert.True(t, stuckGuildIsAdmin(roles, []snowflake.ID{100, 200}, stuckGuildID))
}

func TestStuckGuildIsAdmin_NoAdmin(t *testing.T) {
	roles := []discord.Role{
		roleWith(stuckGuildID, discord.PermissionViewChannel),
		roleWith(100, discord.PermissionSendMessages|discord.PermissionManageMessages),
	}
	assert.False(t, stuckGuildIsAdmin(roles, []snowflake.ID{100}, stuckGuildID))
}

func TestStuckGuildIsAdmin_UnknownRoleIDIsSkipped(t *testing.T) {
	// A role ID present on the member but missing from the guild's role
	// list (stale member payload, role just deleted, etc.) must not crash
	// the helper or grant unintended perms.
	roles := []discord.Role{
		roleWith(stuckGuildID, 0),
		roleWith(100, discord.PermissionSendMessages),
	}
	assert.False(t, stuckGuildIsAdmin(roles, []snowflake.ID{100, 999}, stuckGuildID))
}

func TestStuckGuildIsAdmin_EveryoneRoleMissing(t *testing.T) {
	// If the @everyone role isn't in the slice, the helper must default
	// its perms to zero rather than panicking on a missing map key.
	roles := []discord.Role{
		roleWith(100, discord.PermissionAdministrator),
	}
	assert.True(t, stuckGuildIsAdmin(roles, []snowflake.ID{100}, stuckGuildID))

	rolesNoAdmin := []discord.Role{
		roleWith(100, discord.PermissionSendMessages),
	}
	assert.False(t, stuckGuildIsAdmin(rolesNoAdmin, []snowflake.ID{100}, stuckGuildID))
}

func TestStuckGuildIsAdmin_NoMemberRoles(t *testing.T) {
	roles := []discord.Role{
		roleWith(stuckGuildID, discord.PermissionSendMessages),
	}
	assert.False(t, stuckGuildIsAdmin(roles, nil, stuckGuildID))
}

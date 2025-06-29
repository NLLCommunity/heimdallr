package utils

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
)

func TestHasRole(t *testing.T) {
	role1 := snowflake.ID(111)
	role2 := snowflake.ID(222)
	role3 := snowflake.ID(333)

	member := discord.Member{
		RoleIDs: []snowflake.ID{role1, role2},
	}

	tests := []struct {
		name     string
		member   discord.Member
		roleID   snowflake.ID
		expected bool
	}{
		{
			name:     "has role",
			member:   member,
			roleID:   role1,
			expected: true,
		},
		{
			name:     "has second role",
			member:   member,
			roleID:   role2,
			expected: true,
		},
		{
			name:     "does not have role",
			member:   member,
			roleID:   role3,
			expected: false,
		},
		{
			name: "member with no roles",
			member: discord.Member{
				RoleIDs: []snowflake.ID{},
			},
			roleID:   role1,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasRole(tt.member, tt.roleID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasRolesAll(t *testing.T) {
	role1 := snowflake.ID(111)
	role2 := snowflake.ID(222)
	role3 := snowflake.ID(333)
	role4 := snowflake.ID(444)

	member := discord.Member{
		RoleIDs: []snowflake.ID{role1, role2, role3},
	}

	tests := []struct {
		name     string
		member   discord.Member
		roleIDs  []snowflake.ID
		expected bool
	}{
		{
			name:     "has all roles - single",
			member:   member,
			roleIDs:  []snowflake.ID{role1},
			expected: true,
		},
		{
			name:     "has all roles - multiple",
			member:   member,
			roleIDs:  []snowflake.ID{role1, role2},
			expected: true,
		},
		{
			name:     "has all roles - all",
			member:   member,
			roleIDs:  []snowflake.ID{role1, role2, role3},
			expected: true,
		},
		{
			name:     "missing one role",
			member:   member,
			roleIDs:  []snowflake.ID{role1, role4},
			expected: false,
		},
		{
			name:     "missing all roles",
			member:   member,
			roleIDs:  []snowflake.ID{role4},
			expected: false,
		},
		{
			name:     "empty roles check",
			member:   member,
			roleIDs:  []snowflake.ID{},
			expected: true, // Should be true for empty set.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasRolesAll(tt.member, tt.roleIDs...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasRolesAny(t *testing.T) {
	role1 := snowflake.ID(111)
	role2 := snowflake.ID(222)
	role3 := snowflake.ID(333)
	role4 := snowflake.ID(444)

	member := discord.Member{
		RoleIDs: []snowflake.ID{role1, role2},
	}

	tests := []struct {
		name     string
		member   discord.Member
		roleIDs  []snowflake.ID
		expected bool
	}{
		{
			name:     "has one of the roles",
			member:   member,
			roleIDs:  []snowflake.ID{role1, role3, role4},
			expected: true,
		},
		{
			name:     "has multiple of the roles",
			member:   member,
			roleIDs:  []snowflake.ID{role1, role2},
			expected: true,
		},
		{
			name:     "has none of the roles",
			member:   member,
			roleIDs:  []snowflake.ID{role3, role4},
			expected: false,
		},
		{
			name:     "empty roles check",
			member:   member,
			roleIDs:  []snowflake.ID{},
			expected: false, // Should be false for empty set.
		},
		{
			name: "member with no roles",
			member: discord.Member{
				RoleIDs: []snowflake.ID{},
			},
			roleIDs:  []snowflake.ID{role1, role2},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasRolesAny(tt.member, tt.roleIDs...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

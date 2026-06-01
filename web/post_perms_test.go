package web

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"

	"github.com/NLLCommunity/heimdallr/model"
)

const (
	testUserA   snowflake.ID = 1
	testRoleA   snowflake.ID = 100
	testRoleB   snowflake.ID = 101
	testGuildID snowflake.ID = 99
)

func memberWithRoles(roleIDs ...snowflake.ID) *discord.Member {
	return &discord.Member{
		User:    discord.User{ID: testUserA},
		RoleIDs: roleIDs,
	}
}

func TestHasPostsModRole_NilMember(t *testing.T) {
	s := &model.GuildSettings{PostsModRoleID: testRoleA}
	assert.False(t, hasPostsModRole(s, nil))
}

func TestHasPostsModRole_NilSettings(t *testing.T) {
	// nil settings must not panic and must return false. Defensive
	// against callers that didn't load settings before checking.
	assert.False(t, hasPostsModRole(nil, memberWithRoles(testRoleA)))
}

func TestHasPostsModRole_NoRoleConfigured(t *testing.T) {
	// When no role is configured the answer is always false regardless
	// of which roles the member holds — admins must opt in explicitly.
	s := &model.GuildSettings{PostsModRoleID: 0}
	assert.False(t, hasPostsModRole(s, memberWithRoles(testRoleA)))
}

func TestHasPostsModRole_MemberHasRole(t *testing.T) {
	s := &model.GuildSettings{PostsModRoleID: testRoleA}
	assert.True(t, hasPostsModRole(s, memberWithRoles(testRoleA, testRoleB)))
}

func TestHasPostsModRole_MemberLacksRole(t *testing.T) {
	s := &model.GuildSettings{PostsModRoleID: testRoleA}
	assert.False(t, hasPostsModRole(s, memberWithRoles(testRoleB)))
}

func TestHasPostsModRole_EmptyRoleList(t *testing.T) {
	s := &model.GuildSettings{PostsModRoleID: testRoleA}
	assert.False(t, hasPostsModRole(s, memberWithRoles()))
}

// @everyone's role ID equals the guild ID, but Discord never lists it in
// member.RoleIDs. Storing PostsModRoleID == guildID would therefore look
// like "grant access to everyone" but actually grants it to no one.
// Both the web settings handler and the slash command refuse it at save
// time, so this case shouldn't appear in practice — but if a legacy row
// ever slips through, the check below documents the resulting behaviour
// (deny, not allow) so future contributors don't paper over it.
func TestHasPostsModRole_EveryoneIsNotAMatch(t *testing.T) {
	s := &model.GuildSettings{PostsModRoleID: testGuildID}
	// Member with no roles — @everyone is implicit, but RoleIDs is empty.
	assert.False(t, hasPostsModRole(s, memberWithRoles()))
	// Member with other roles — still no match.
	assert.False(t, hasPostsModRole(s, memberWithRoles(testRoleA, testRoleB)))
}

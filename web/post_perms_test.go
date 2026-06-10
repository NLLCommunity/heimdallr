package web

import (
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
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
	assert.False(t, hasPostsModRole(testRoleA, nil))
}

func TestHasPostsModRole_NoRoleConfigured(t *testing.T) {
	// When no role is configured the answer is always false regardless
	// of which roles the member holds - admins must opt in explicitly.
	assert.False(t, hasPostsModRole(0, memberWithRoles(testRoleA)))
}

func TestHasPostsModRole_MemberHasRole(t *testing.T) {
	assert.True(t, hasPostsModRole(testRoleA, memberWithRoles(testRoleA, testRoleB)))
}

func TestHasPostsModRole_MemberLacksRole(t *testing.T) {
	assert.False(t, hasPostsModRole(testRoleA, memberWithRoles(testRoleB)))
}

func TestHasPostsModRole_EmptyRoleList(t *testing.T) {
	assert.False(t, hasPostsModRole(testRoleA, memberWithRoles()))
}

// A timed-out (communication-disabled) member must lose posts access for
// the duration of the timeout: Discord strips their permissions, and the
// dashboard gate has to match instead of letting a muted moderator keep
// publishing posts through the bot.
func TestHasPostsModRole_TimedOutMemberDenied(t *testing.T) {
	member := memberWithRoles(testRoleA)
	until := time.Now().Add(time.Hour)
	member.CommunicationDisabledUntil = &until
	assert.False(t, hasPostsModRole(testRoleA, member))
}

// An expired timeout must not keep denying access - only a timestamp in
// the future counts as communication-disabled.
func TestHasPostsModRole_ExpiredTimeoutAllowed(t *testing.T) {
	member := memberWithRoles(testRoleA)
	until := time.Now().Add(-time.Hour)
	member.CommunicationDisabledUntil = &until
	assert.True(t, hasPostsModRole(testRoleA, member))
}

// @everyone's role ID equals the guild ID, but Discord never lists it in
// member.RoleIDs. Storing PostsModRoleID == guildID would therefore look
// like "grant access to everyone" but actually grants it to no one.
// Both the web settings handler and the slash command refuse it at save
// time via model.ValidatePostsModRole, so this case shouldn't appear in
// practice - but if a legacy row ever slips through, the check below
// documents the resulting behaviour (deny, not allow) so future
// contributors don't paper over it.
func TestHasPostsModRole_EveryoneIsNotAMatch(t *testing.T) {
	// Member with no roles - @everyone is implicit, but RoleIDs is empty.
	assert.False(t, hasPostsModRole(testGuildID, memberWithRoles()))
	// Member with other roles - still no match.
	assert.False(t, hasPostsModRole(testGuildID, memberWithRoles(testRoleA, testRoleB)))
}

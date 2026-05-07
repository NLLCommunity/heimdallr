package web

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
)

// helpers
func roleOverride(roleID snowflake.ID, allow bool) discord.ApplicationCommandPermission {
	return discord.ApplicationCommandPermissionRole{
		RoleID:     roleID,
		Permission: allow,
	}
}
func userOverride(userID snowflake.ID, allow bool) discord.ApplicationCommandPermission {
	return discord.ApplicationCommandPermissionUser{
		UserID:     userID,
		Permission: allow,
	}
}

const (
	testUserA   snowflake.ID = 1
	testRoleA   snowflake.ID = 100
	testRoleB   snowflake.ID = 101
	testGuildID snowflake.ID = 99
)

func TestResolveCommandPermission_NoOverridesUsesDefault(t *testing.T) {
	allow := resolveCommandPermission(nil, testUserA, []snowflake.ID{testRoleA}, testGuildID, true)
	assert.True(t, allow)

	allow = resolveCommandPermission(nil, testUserA, []snowflake.ID{testRoleA}, testGuildID, false)
	assert.False(t, allow)
}

func TestResolveCommandPermission_UserOverrideBeatsRole(t *testing.T) {
	overrides := []discord.ApplicationCommandPermission{
		roleOverride(testRoleA, false),
		userOverride(testUserA, true),
	}
	allow := resolveCommandPermission(overrides, testUserA, []snowflake.ID{testRoleA}, testGuildID, false)
	assert.True(t, allow)
}

func TestResolveCommandPermission_UserDenyBeatsRoleAllow(t *testing.T) {
	overrides := []discord.ApplicationCommandPermission{
		roleOverride(testRoleA, true),
		userOverride(testUserA, false),
	}
	allow := resolveCommandPermission(overrides, testUserA, []snowflake.ID{testRoleA}, testGuildID, true)
	assert.False(t, allow)
}

func TestResolveCommandPermission_AnyAllowingRoleAllows(t *testing.T) {
	overrides := []discord.ApplicationCommandPermission{
		roleOverride(testRoleA, false),
		roleOverride(testRoleB, true),
	}
	allow := resolveCommandPermission(overrides, testUserA, []snowflake.ID{testRoleA, testRoleB}, testGuildID, false)
	assert.True(t, allow)
}

func TestResolveCommandPermission_AllRolesDeny(t *testing.T) {
	overrides := []discord.ApplicationCommandPermission{
		roleOverride(testRoleA, false),
		roleOverride(testRoleB, false),
	}
	allow := resolveCommandPermission(overrides, testUserA, []snowflake.ID{testRoleA, testRoleB}, testGuildID, true)
	assert.False(t, allow)
}

func TestResolveCommandPermission_EveryoneRoleApplies(t *testing.T) {
	overrides := []discord.ApplicationCommandPermission{
		roleOverride(testGuildID, false),
	}
	allow := resolveCommandPermission(overrides, testUserA, nil, testGuildID, true)
	assert.False(t, allow)
}

func TestCommandOverrideCache_HitWithinTTL(t *testing.T) {
	c := newCommandOverrideCache(50 * time.Millisecond)
	calls := int64(0)
	fetch := func() ([]discord.ApplicationCommandPermission, error) {
		atomic.AddInt64(&calls, 1)
		return []discord.ApplicationCommandPermission{}, nil
	}
	for i := 0; i < 5; i++ {
		_, err := c.get(testGuildID, fetch)
		assert.NoError(t, err)
	}
	assert.EqualValues(t, 1, atomic.LoadInt64(&calls))
}

func TestCommandOverrideCache_RefetchesAfterTTL(t *testing.T) {
	c := newCommandOverrideCache(20 * time.Millisecond)
	calls := int64(0)
	fetch := func() ([]discord.ApplicationCommandPermission, error) {
		atomic.AddInt64(&calls, 1)
		return nil, nil
	}
	_, _ = c.get(testGuildID, fetch)
	time.Sleep(30 * time.Millisecond)
	_, _ = c.get(testGuildID, fetch)
	assert.EqualValues(t, 2, atomic.LoadInt64(&calls))
}

func TestCommandOverrideCache_InvalidateForcesRefetch(t *testing.T) {
	c := newCommandOverrideCache(time.Hour)
	calls := int64(0)
	fetch := func() ([]discord.ApplicationCommandPermission, error) {
		atomic.AddInt64(&calls, 1)
		return nil, nil
	}
	_, _ = c.get(testGuildID, fetch)
	c.invalidate(testGuildID)
	_, _ = c.get(testGuildID, fetch)
	assert.EqualValues(t, 2, atomic.LoadInt64(&calls))
}

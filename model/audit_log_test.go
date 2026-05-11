package model

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *ModelTestSuite) seedAuditEntries() (snowflake.ID, []uint) {
	guildID := snowflake.ID(123)
	actor := snowflake.ID(7777)
	target := snowflake.ID(8888)
	otherUser := snowflake.ID(9999)

	now := time.Now().UTC()
	entries := []AuditLogEntry{
		{GuildID: guildID, Category: "message", EventType: "message.delete", ActorID: &actor, ActorKind: "user", TargetID: &target, TargetKind: "message", Source: "gateway", CreatedAt: now.Add(-30 * time.Minute)},
		{GuildID: guildID, Category: "member", EventType: "member.nick_change", ActorID: &target, ActorKind: "user", TargetID: &target, TargetKind: "user", Source: "gateway", CreatedAt: now.Add(-20 * time.Minute)},
		{GuildID: guildID, Category: "guild", EventType: "guild.ban", ActorID: &actor, ActorKind: "user", TargetID: &otherUser, TargetKind: "user", Source: "gateway", CreatedAt: now.Add(-10 * time.Minute)},
		{GuildID: snowflake.ID(456), Category: "guild", EventType: "guild.ban", ActorID: &actor, ActorKind: "user", TargetID: &otherUser, TargetKind: "user", Source: "gateway", CreatedAt: now},
	}

	ids := make([]uint, 0, len(entries))
	for i := range entries {
		require.NoError(suite.T(), suite.db.Create(&entries[i]).Error)
		ids = append(ids, entries[i].ID)
	}
	return guildID, ids
}

func (suite *ModelTestSuite) TestListAuditLogEntries_GuildScope() {
	guildID, _ := suite.seedAuditEntries()

	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(3), count)
	assert.Len(suite.T(), entries, 3)
	// Verify newest-first ordering.
	for i := 1; i < len(entries); i++ {
		assert.True(suite.T(),
			!entries[i-1].CreatedAt.Before(entries[i].CreatedAt),
			"entries must be newest-first")
	}
}

func (suite *ModelTestSuite) TestListAuditLogEntries_FilterCategory() {
	guildID, _ := suite.seedAuditEntries()

	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{Category: "message"}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)
	assert.Len(suite.T(), entries, 1)
	assert.Equal(suite.T(), "message.delete", entries[0].EventType)
}

func (suite *ModelTestSuite) TestListAuditLogEntries_FilterActor() {
	guildID, _ := suite.seedAuditEntries()
	actor := snowflake.ID(7777)

	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{ActorIDs: []snowflake.ID{actor}}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(2), count)
	assert.Len(suite.T(), entries, 2)
}

func (suite *ModelTestSuite) TestListAuditLogEntries_FilterActorQuery() {
	guildID := snowflake.ID(321)
	actor := snowflake.ID(7777)
	other := snowflake.ID(8888)

	require.NoError(suite.T(), suite.db.Create(&AuditLogEntry{
		GuildID: guildID, Category: "guild", EventType: "guild.ban",
		ActorID: &actor, ActorKind: "user",
		TargetID: &other, TargetKind: "user", Source: "gateway",
		Details: `{"actor_username":"myrkvi","target_username":"someoneElse"}`,
	}).Error)
	require.NoError(suite.T(), suite.db.Create(&AuditLogEntry{
		GuildID: guildID, Category: "member", EventType: "member.nick_change",
		ActorID: &other, ActorKind: "user",
		TargetID: &other, TargetKind: "user", Source: "gateway",
		Details: `{"actor_username":"someoneElse","target_username":"someoneElse"}`,
	}).Error)

	// LIKE on actor_username "myrk" matches the first row only, even
	// though no IDs were resolved by the (mocked) cache.
	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{ActorQuery: "myrk"}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)
	assert.Len(suite.T(), entries, 1)
	assert.Equal(suite.T(), "guild.ban", entries[0].EventType)
}

func (suite *ModelTestSuite) TestListAuditLogEntries_FilterActorMulti() {
	guildID, _ := suite.seedAuditEntries()
	// Two distinct actors — should match both via IN clause.
	a1 := snowflake.ID(7777)
	a2 := snowflake.ID(8888)

	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{ActorIDs: []snowflake.ID{a1, a2}}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(3), count, "actor 7777 has 2 rows, actor 8888 has 1 — IN should sum to 3")
	assert.Len(suite.T(), entries, 3)
}

func (suite *ModelTestSuite) TestListAuditLogEntries_TimeRange() {
	guildID, _ := suite.seedAuditEntries()

	now := time.Now().UTC()
	from := now.Add(-15 * time.Minute)
	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{From: from}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count, "only the entry within the last 15min matches")
	assert.Len(suite.T(), entries, 1)
}

func (suite *ModelTestSuite) TestListAuditLogEntries_Pagination() {
	guildID, _ := suite.seedAuditEntries()

	entries, count, err := ListAuditLogEntries(guildID, AuditLogFilter{}, 2, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(3), count, "total ignores pagination")
	assert.Len(suite.T(), entries, 2)

	entries, _, err = ListAuditLogEntries(guildID, AuditLogFilter{}, 2, 2)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), entries, 1)
}

func (suite *ModelTestSuite) TestPruneAuditLogEntriesBefore() {
	guildID, _ := suite.seedAuditEntries()

	// Cut off everything older than 15 minutes ago.
	cutoff := time.Now().UTC().Add(-15 * time.Minute)

	deleted, err := PruneAuditLogEntriesBefore(context.Background(), guildID, "message", cutoff)
	require.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 1, deleted)

	// Member-category row is older than 15min but not in the message
	// category — should be untouched.
	_, count, err := ListAuditLogEntries(guildID, AuditLogFilter{Category: "member"}, 50, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)
}

func (suite *ModelTestSuite) TestPruneAuditLogEntriesBefore_ZeroCutoff() {
	guildID, _ := suite.seedAuditEntries()

	deleted, err := PruneAuditLogEntriesBefore(context.Background(), guildID, "message", time.Time{})
	require.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 0, deleted, "zero cutoff means no-op")
}

func (suite *ModelTestSuite) TestDistinctAuditLogGuilds() {
	suite.seedAuditEntries()
	guilds, err := DistinctAuditLogGuilds()
	require.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), []snowflake.ID{snowflake.ID(123), snowflake.ID(456)}, guilds)
}

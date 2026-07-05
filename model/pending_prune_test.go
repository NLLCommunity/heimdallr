package model

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func member(guildID, userID snowflake.ID) discord.Member {
	return discord.Member{GuildID: guildID, User: discord.User{ID: userID}}
}

// A user can be queued in two prune batches at once. Marking them pruned in one
// batch must not touch the other batch's row, and cleaning up one batch must
// not delete the other's rows.
func (suite *ModelTestSuite) TestSetMemberPruned_ScopedToBatch() {
	t := suite.T()
	guildID := snowflake.ID(555)
	shared := snowflake.ID(1001) // appears in both batches
	onlyA := snowflake.ID(1002)
	onlyB := snowflake.ID(1003)

	batchA := uuid.New()
	batchB := uuid.New()

	require.NoError(t, AddMembersToBePruned(batchA, []discord.Member{member(guildID, shared), member(guildID, onlyA)}))
	require.NoError(t, AddMembersToBePruned(batchB, []discord.Member{member(guildID, shared), member(guildID, onlyB)}))

	// Mark the shared user pruned in batch A only.
	require.NoError(t, SetMemberPruned(guildID, batchA, shared, true))

	prunedA, err := GetPrunedMembers(batchA, guildID)
	require.NoError(t, err)
	assert.Len(t, prunedA, 1, "batch A should have exactly the shared user pruned")
	assert.Equal(t, shared, prunedA[0].UserID)

	// Batch B's row for the shared user must remain un-pruned.
	prunedB, err := GetPrunedMembers(batchB, guildID)
	require.NoError(t, err)
	assert.Empty(t, prunedB, "batch B must be untouched by batch A's SetMemberPruned")

	toPruneB, err := GetMembersToPrune(batchB, guildID)
	require.NoError(t, err)
	assert.Len(t, toPruneB, 2, "batch B should still list both of its members as pending")

	// Cleaning up batch A must not remove batch B's rows.
	require.NoError(t, RemoveMembersByPruneID(batchA, guildID))
	toPruneBAfter, err := GetMembersToPrune(batchB, guildID)
	require.NoError(t, err)
	assert.Len(t, toPruneBAfter, 2, "batch B rows must survive batch A cleanup")
}

// IsMemberPruned spans batches and must report true when the user is pruned in
// any batch, regardless of row ordering (the old members[0] read could return a
// non-pruned row).
func (suite *ModelTestSuite) TestIsMemberPruned_AnyBatch() {
	t := suite.T()
	guildID := snowflake.ID(777)
	user := snowflake.ID(2001)

	batchA := uuid.New()
	batchB := uuid.New()
	require.NoError(t, AddMembersToBePruned(batchA, []discord.Member{member(guildID, user)}))
	require.NoError(t, AddMembersToBePruned(batchB, []discord.Member{member(guildID, user)}))

	// Not pruned in any batch yet.
	pruned, err := IsMemberPruned(guildID, user)
	require.NoError(t, err)
	assert.False(t, pruned)

	// Pruned in batch B only; must still report true across batches.
	require.NoError(t, SetMemberPruned(guildID, batchB, user, true))
	pruned, err = IsMemberPruned(guildID, user)
	require.NoError(t, err)
	assert.True(t, pruned, "user pruned in any batch must read as pruned")
}

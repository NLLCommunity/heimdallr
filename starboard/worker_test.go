package starboard

import (
	"context"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestQueueCoalescesAndPreservesDeletion(t *testing.T) {
	s, _, _ := setup(t)
	s.Notify(10, 30, 100, false)
	s.Notify(10, 30, 100, true)
	s.Notify(10, 30, 100, false)
	var jobs []model.StarboardJob
	require.NoError(t, s.db.Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.True(t, jobs[0].Deleted)
}
func TestDisabledUntrackedAndUnrelatedEditsDoNotQueue(t *testing.T) {
	s, _, _ := setup(t)
	s.NotifyUpdate(10, 30, 100)
	var count int64
	require.NoError(t, s.db.Model(&model.StarboardJob{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, s.SetEnabled(10, false))
	s.Notify(10, 30, 100, false)
	require.NoError(t, s.db.Model(&model.StarboardJob{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestWorkerReconcilesPersistedJob(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	s.Notify(10, 30, 100, false)
	require.NoError(t, s.db.Model(&model.StarboardJob{}).Where("1=1").Update("next_attempt", time.Now().Add(-time.Second)).Error)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	// Observe only DB state here; transport state is owned by the worker.
	require.Eventually(t, func() bool {
		var n int64
		return s.db.Model(&model.StarboardEntry{}).Where("copy_message_id <> 0").Count(&n).Error == nil && n == 1
	}, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
	require.Len(t, f.published, 1)
}

func TestCopyEditsNeverEnqueueSourceRefresh(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	e := entry(t, s, b)
	s.NotifyUpdate(10, e.CopyChannelID, e.CopyMessageID)
	var n int64
	require.NoError(t, s.db.Model(&model.StarboardJob{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestModeratorRemovalBypassesOldNetworkBackoff(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	s.Notify(10, 30, 100, false)
	require.NoError(t, s.db.Model(&model.StarboardJob{}).Where("1=1").Update("next_attempt", time.Now().Add(15*time.Minute)).Error)
	require.NoError(t, s.Moderate(10, b.ID, "remove", 30, 100))
	var job model.StarboardJob
	require.NoError(t, s.db.First(&job).Error)
	require.WithinDuration(t, time.Now(), job.NextAttempt, 2*time.Second)
}

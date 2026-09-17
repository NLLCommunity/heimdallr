package starboard

import (
	"testing"
	"time"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
)

type recoveringTransport struct {
	*fakeTransport
	real *discordTransport
}

func (f *recoveringTransport) Recover(b model.Starboard, n string, since time.Time, before snowflake.ID) (snowflake.ID, snowflake.ID, error) {
	return f.real.Recover(b, n, since, before)
}

func TestRecoveryProgressSurvivesRestartAndReachesOlderCopy(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	f.failPublish = true
	require.Error(t, s.reconcile(10, 30, 100, false, true))
	e := entry(t, s, b)
	since := time.Now().Add(-time.Hour)
	e.SendStartedAt = &since
	require.NoError(t, s.db.Save(&e).Error)
	start := snowflake.New(since)
	target := start + 1
	messages := make([]discord.Message, 1001)
	for i := range messages {
		messages[i] = discord.Message{ID: start + snowflake.ID(1001-i), Author: discord.User{ID: 30}}
	}
	messages[1000].Embeds = []discord.Embed{{URL: recoveryMarker("https://discord.com/channels/10/30/100", e.SendNonce)}}
	var calls int
	restFake := recoveryPermissionFake(t, discord.PermissionViewChannel|discord.PermissionReadMessageHistory)
	restFake.getMessages = func(_ snowflake.ID, _, before, _ snowflake.ID, limit int) ([]discord.Message, error) {
		calls++
		var page []discord.Message
		for _, m := range messages {
			if before == 0 || m.ID < before {
				page = append(page, m)
				if len(page) == limit {
					break
				}
			}
		}
		return page, nil
	}
	real := newDiscordTransportWithREST(nil, restFake)
	real.selfID = func() snowflake.ID { return 30 }
	transport := &recoveringTransport{fakeTransport: f, real: real}
	s = New(s.db, transport)
	require.ErrorIs(t, s.reconcile(10, 30, 100, false, false), ErrAmbiguousSend)
	require.Equal(t, 10, calls, "one pass must stay bounded")
	require.NotZero(t, entry(t, s, b).RecoveryBefore)
	// A fresh service/transport must use the persisted cursor, not memory.
	real = newDiscordTransportWithREST(nil, restFake)
	real.selfID = func() snowflake.ID { return 30 }
	s = New(s.db, &recoveringTransport{fakeTransport: f, real: real})
	require.NoError(t, s.reconcile(10, 30, 100, false, false))
	require.Equal(t, target, entry(t, s, b).CopyMessageID)
	require.Zero(t, entry(t, s, b).RecoveryBefore)
	require.Equal(t, 11, calls)
	require.Len(t, f.published, 1)
}

func TestRecoveryRetainsUnreadCursorWhenPermissionIsMissing(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	before := snowflake.New(since.Add(time.Minute))
	fake := recoveryPermissionFake(t, discord.PermissionViewChannel)
	real := newDiscordTransportWithREST(nil, fake)
	real.selfID = func() snowflake.ID { return 30 }
	id, next, err := real.Recover(model.Starboard{GuildID: 10, ChannelID: 20}, "pending", since, before)
	require.NoError(t, err)
	require.Zero(t, id)
	require.Equal(t, before, next, "an empty inaccessible page must not advance the search")
}

func TestRecentRecoveryRestartsToFindLateSend(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	start := snowflake.New(since)
	fake := &fakeDiscordREST{getMessages: func(_ snowflake.ID, _, before, _ snowflake.ID, _ int) ([]discord.Message, error) {
		require.Zero(t, before, "do not skip a late send in a recently scanned range")
		return []discord.Message{{ID: start + 500, Author: discord.User{ID: 30}, Embeds: []discord.Embed{{URL: recoveryMarker("https://discord.com/channels/10/30/100", "pending")}}}}, nil
	}}
	real := newDiscordTransportWithREST(nil, fake)
	real.selfID = func() snowflake.ID { return 30 }
	id, next, err := real.Recover(model.Starboard{GuildID: 10, ChannelID: 20}, "pending", since, start+100)
	require.NoError(t, err)
	require.Equal(t, start+500, id)
	require.Zero(t, next)
}

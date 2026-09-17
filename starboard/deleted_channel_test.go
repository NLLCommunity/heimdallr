package starboard

import (
	"testing"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
)

type goneChannelTransport struct {
	*fakeTransport
	real       *discordTransport
	sourceGone bool
}

func (f *goneChannelTransport) Source(g, c, m snowflake.ID) (*discord.Message, error) {
	if f.sourceGone {
		return f.real.Source(g, c, m)
	}
	return f.fakeTransport.Source(g, c, m)
}
func (f *goneChannelTransport) Delete(c, m snowflake.ID) error {
	if !f.sourceGone {
		return f.real.Delete(c, m)
	}
	return f.fakeTransport.Delete(c, m)
}
func TestDeletedSourceChannelRemovesCopy(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	real := newDiscordTransportWithREST(nil, &fakeDiscordREST{getChannel: func(snowflake.ID) (discord.Channel, error) {
		return nil, &rest.Error{Code: rest.JSONErrorCodeUnknownChannel, Message: "Unknown Channel"}
	}})
	s.transport = &goneChannelTransport{fakeTransport: f, real: real, sourceGone: true}
	require.NoError(t, s.reconcile(10, 30, 100, false, false))
	e := entry(t, s, b)
	require.True(t, e.SourceDeleted)
	require.Zero(t, e.CopyMessageID, "a source channel was deleted, so its copy should disappear")
}
func TestDeletedDestinationAllowsBoardDeletion(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	real := newDiscordTransportWithREST(nil, &fakeDiscordREST{deleteMessage: func(snowflake.ID, snowflake.ID) error {
		return &rest.Error{Code: rest.JSONErrorCodeUnknownChannel, Message: "Unknown Channel"}
	}})
	s.transport = &goneChannelTransport{fakeTransport: f, real: real}
	require.NoError(t, s.DeleteBoard(10, b.ID))
	for i := 0; i < 3; i++ {
		require.NoError(t, s.reconcile(10, 30, 100, false, false))
		s.finishDeletedBoards()
	}
	var remaining int64
	require.NoError(t, s.db.Model(&model.Starboard{}).Where("id = ?", b.ID).Count(&remaining).Error)
	require.Zero(t, remaining, "deleted destination has no copies left to clean up")
}

func TestDeletedDestinationAllowsMigration(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	real := newDiscordTransportWithREST(nil, &fakeDiscordREST{deleteMessage: func(snowflake.ID, snowflake.ID) error { return &rest.Error{Code: rest.JSONErrorCodeUnknownChannel} }})
	s.transport = &goneChannelTransport{fakeTransport: f, real: real}
	b.ChannelID = 21
	require.NoError(t, s.SaveBoard(&b))
	require.NoError(t, s.reconcile(10, 30, 100, false, false))
	require.Equal(t, snowflake.ID(21), entry(t, s, b).CopyChannelID)
	require.Len(t, f.published, 2)
}

func TestChannelPermissionFailuresRemainRetryable(t *testing.T) {
	denied := &rest.Error{Code: rest.JSONErrorCodeMissingAccess}
	real := newDiscordTransportWithREST(nil, &fakeDiscordREST{
		getChannel:    func(snowflake.ID) (discord.Channel, error) { return nil, denied },
		deleteMessage: func(snowflake.ID, snowflake.ID) error { return denied },
	})
	_, err := real.Source(10, 30, 100)
	require.ErrorIs(t, err, denied)
	require.NotErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, real.Delete(20, 1000), denied)
}

func TestDeletedDestinationClearsAmbiguousSendBeforeBoardDeletion(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	f.failPublish = true
	require.Error(t, s.reconcile(10, 30, 100, false, true))
	real := newDiscordTransportWithREST(nil, &fakeDiscordREST{getMessages: func(snowflake.ID, snowflake.ID, snowflake.ID, snowflake.ID, int) ([]discord.Message, error) {
		return nil, &rest.Error{Code: rest.JSONErrorCodeUnknownChannel}
	}})
	real.selfID = func() snowflake.ID { return 30 }
	s.transport = &recoveringTransport{fakeTransport: f, real: real}
	require.NoError(t, s.DeleteBoard(10, b.ID))
	require.ErrorIs(t, s.reconcile(10, 30, 100, false, false), ErrNotFound)
	require.Empty(t, entry(t, s, b).SendNonce)
	require.NoError(t, s.reconcile(10, 30, 100, false, false))
	s.finishDeletedBoards()
	var remaining int64
	require.NoError(t, s.db.Model(&model.Starboard{}).Where("id = ?", b.ID).Count(&remaining).Error)
	require.Zero(t, remaining)
	require.Len(t, f.published, 1)
}

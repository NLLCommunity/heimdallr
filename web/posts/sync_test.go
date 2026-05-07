package posts

import (
	"errors"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
)

// fakeDiscord implements DiscordClient for tests; records calls so we can
// assert on the sequence of operations.
type fakeDiscord struct {
	sent      []sentMsg
	edited    []editMsg
	deleted   []snowflake.ID
	sendErr   error
	editErr   error
	deleteErr error
	nextID    uint64
}

type sentMsg struct {
	channelID snowflake.ID
	chunk     []any
}
type editMsg struct {
	channelID snowflake.ID
	messageID snowflake.ID
	chunk     []any
}

func (f *fakeDiscord) SendV2(channelID snowflake.ID, chunk []any) (snowflake.ID, error) {
	if f.sendErr != nil {
		return 0, f.sendErr
	}
	f.sent = append(f.sent, sentMsg{channelID, chunk})
	f.nextID++
	return snowflake.ID(f.nextID), nil
}
func (f *fakeDiscord) EditV2(channelID, messageID snowflake.ID, chunk []any) error {
	if f.editErr != nil {
		return f.editErr
	}
	f.edited = append(f.edited, editMsg{channelID, messageID, chunk})
	return nil
}
func (f *fakeDiscord) Delete(channelID, messageID snowflake.ID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, messageID)
	return nil
}

func TestSync_FirstPublishCreatesAllChunks(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "1"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "2"}},
	}
	target := snowflake.ID(42)
	fd := &fakeDiscord{}

	plan := SyncPlan{NewChunks: chunks, ChannelID: target}
	result, err := Sync(fd, plan, nil)
	assert.NoError(t, err)
	assert.Len(t, fd.sent, 2)
	assert.Empty(t, fd.edited)
	assert.Empty(t, fd.deleted)
	assert.Len(t, result.Created, 2)
	assert.EqualValues(t, target, result.Created[0].ChannelID)
}

func TestSync_NoOpForEmptyPlanAndNoExisting(t *testing.T) {
	fd := &fakeDiscord{}
	result, err := Sync(fd, SyncPlan{NewChunks: nil, ChannelID: 1}, nil)
	assert.NoError(t, err)
	assert.Empty(t, fd.sent)
	assert.Empty(t, fd.edited)
	assert.Empty(t, fd.deleted)
	assert.Empty(t, result.Created)
}

func TestSync_FirstPublishAbortsOnSendError(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "1"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "2"}},
	}
	fd := &fakeDiscord{sendErr: errors.New("rate limited")}
	_, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 42}, nil)
	assert.Error(t, err)
}

func TestSync_EqualLengthEditsInPlace(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "new1"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "new2"}},
	}
	existing := []ExistingMessage{
		{ChannelID: 42, MessageID: 1001},
		{ChannelID: 42, MessageID: 1002},
	}
	fd := &fakeDiscord{}

	result, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 42}, existing)
	assert.NoError(t, err)
	assert.Len(t, fd.edited, 2)
	assert.Empty(t, fd.sent)
	assert.Empty(t, fd.deleted)
	assert.EqualValues(t, 1001, fd.edited[0].messageID)
	assert.EqualValues(t, 1002, fd.edited[1].messageID)
	assert.Equal(t, 2, result.KeptCount)
}

func TestSync_FewerChunksDeletesTrailing(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "kept1"}},
	}
	existing := []ExistingMessage{
		{ChannelID: 42, MessageID: 1001},
		{ChannelID: 42, MessageID: 1002},
		{ChannelID: 42, MessageID: 1003},
	}
	fd := &fakeDiscord{}

	result, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 42}, existing)
	assert.NoError(t, err)
	assert.Len(t, fd.edited, 1)
	assert.EqualValues(t, 1001, fd.edited[0].messageID)
	assert.ElementsMatch(t, []snowflake.ID{1002, 1003}, fd.deleted)
	assert.Equal(t, 1, result.KeptCount)
	assert.Equal(t, 2, result.DeletedCount)
}

func TestSync_FewerChunks_DeleteFailureIsSwallowed(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "kept1"}},
	}
	existing := []ExistingMessage{
		{ChannelID: 42, MessageID: 1001},
		{ChannelID: 42, MessageID: 1002},
	}
	fd := &fakeDiscord{deleteErr: errors.New("unknown message")}
	result, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 42}, existing)
	assert.NoError(t, err)
	assert.Equal(t, 1, result.DeletedCount)
}

func TestSync_MoreChunksRecreatesAll(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "n1"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "n2"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "n3"}},
	}
	existing := []ExistingMessage{
		{ChannelID: 42, MessageID: 1001},
		{ChannelID: 42, MessageID: 1002},
	}
	fd := &fakeDiscord{}
	result, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 42}, existing)
	assert.NoError(t, err)
	assert.Empty(t, fd.edited)
	assert.ElementsMatch(t, []snowflake.ID{1001, 1002}, fd.deleted)
	assert.Len(t, fd.sent, 3)
	assert.True(t, result.RecreatedAll)
	assert.Len(t, result.Created, 3)
}

func TestSync_RecreateAllAbortsMidwayWithPartialCreated(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "n1"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "n2"}},
		{map[string]any{"type": float64(typeTextDisplay), "content": "n3"}},
	}
	existing := []ExistingMessage{
		{ChannelID: 42, MessageID: 1001},
		{ChannelID: 42, MessageID: 1002},
	}
	// Fail on the third send only.
	fd := &fakeDiscordFailNthSend{n: 3}
	result, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 42}, existing)
	assert.Error(t, err)
	assert.True(t, result.RecreatedAll)
	assert.Len(t, result.Created, 2) // first two succeeded
}

// fakeDiscordFailNthSend wraps fakeDiscord, but fails on the Nth Send call.
type fakeDiscordFailNthSend struct {
	fakeDiscord
	n     int
	calls int
}

func (f *fakeDiscordFailNthSend) SendV2(channelID snowflake.ID, chunk []any) (snowflake.ID, error) {
	f.calls++
	if f.calls == f.n {
		return 0, errors.New("rate limited")
	}
	return f.fakeDiscord.SendV2(channelID, chunk)
}

func TestSync_ReTargetTriggersFullRecreate(t *testing.T) {
	chunks := [][]any{
		{map[string]any{"type": float64(typeTextDisplay), "content": "x"}},
	}
	existing := []ExistingMessage{
		{ChannelID: 11, MessageID: 1001},
		{ChannelID: 11, MessageID: 1002},
	}
	fd := &fakeDiscord{}
	result, err := Sync(fd, SyncPlan{NewChunks: chunks, ChannelID: 22}, existing)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []snowflake.ID{1001, 1002}, fd.deleted)
	assert.Len(t, fd.sent, 1)
	assert.EqualValues(t, 22, fd.sent[0].channelID)
	assert.True(t, result.RecreatedAll)
	assert.Len(t, result.Created, 1)
}

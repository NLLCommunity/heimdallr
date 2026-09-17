package starboard

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeTransport struct {
	validationError error
	partialPublish  bool
	unsent          bool
	votes           map[string]Votes
	messages        map[snowflake.ID]*discord.Message
	published       []model.Starboard
	deleted         []snowflake.ID
	updated         []snowflake.ID
	next            snowflake.ID
	failPublish     bool
	failDelete      bool
	failSource      bool
	recovered       snowflake.ID
	eligible        bool
	parent          snowflake.ID
}

func voteKey(m snowflake.ID, emoji string) string            { return fmt.Sprintf("%d:%s", m, emoji) }
func (f *fakeTransport) ValidateBoard(model.Starboard) error { return f.validationError }
func (f *fakeTransport) Source(g, c, m snowflake.ID) (*discord.Message, error) {
	if f.failSource {
		return nil, errors.New("forbidden")
	}
	if msg := f.messages[m]; msg != nil {
		return msg, nil
	}
	return nil, ErrNotFound
}
func (f *fakeTransport) Eligible(model.Starboard, *discord.Message) (bool, snowflake.ID, error) {
	return f.eligible, f.parent, nil
}
func (f *fakeTransport) Votes(c, m snowflake.ID, emoji string) (Votes, error) {
	return f.votes[voteKey(m, emoji)], nil
}
func (f *fakeTransport) Publish(b model.Starboard, m *discord.Message, v Votes, n string) (snowflake.ID, error) {
	f.published = append(f.published, b)
	if f.unsent {
		return 0, ErrNotSent
	}
	if f.failPublish {
		return 0, errors.New("connection lost")
	}
	f.next++
	if f.partialPublish {
		return f.next, errors.New("reaction seed failed")
	}
	return f.next, nil
}
func (f *fakeTransport) Recover(model.Starboard, string, time.Time) (snowflake.ID, error) {
	return f.recovered, nil
}
func (f *fakeTransport) Update(b model.Starboard, m *discord.Message, id snowflake.ID, v Votes) error {
	f.updated = append(f.updated, id)
	return nil
}
func (f *fakeTransport) Delete(c, m snowflake.ID) error {
	if f.failDelete {
		return errors.New("unavailable")
	}
	f.deleted = append(f.deleted, m)
	return nil
}
func setup(t *testing.T) (*Service, *fakeTransport, model.Starboard) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.GuildSettings{}, &model.Starboard{}, &model.StarboardEntry{}, &model.StarboardExclusion{}, &model.StarboardJob{}, &model.StarboardCleanup{}))
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	f := &fakeTransport{votes: map[string]Votes{}, messages: map[snowflake.ID]*discord.Message{100: {ID: 100, ChannelID: 30, Content: "hello", Author: discord.User{ID: 50}}}, next: 1000, eligible: true}
	s := New(db, f)
	require.NoError(t, s.SetEnabled(10, true))
	b := model.Starboard{GuildID: 10, Name: "Stars", ChannelID: 20, Emoji: "⭐", Threshold: 4, Enabled: true}
	require.NoError(t, s.SaveBoard(&b))
	return s, f, b
}
func entry(t *testing.T, s *Service, b model.Starboard) model.StarboardEntry {
	t.Helper()
	var e model.StarboardEntry
	require.NoError(t, s.db.Where("board_id = ? AND message_id = ?", b.ID, 100).First(&e).Error)
	return e
}
func reconcile(t *testing.T, s *Service) {
	t.Helper()
	require.NoError(t, s.reconcile(10, 30, 100, false, true))
}
func TestParseEmoji(t *testing.T) {
	for _, raw := range []string{"❌", "❌️", "", "hello", "⭐⭐", "1", ":star:"} {
		_, err := ParseEmoji(raw)
		require.Error(t, err, raw)
	}
	for _, raw := range []string{"⭐", "❤️", "👍🏽", "👨‍👩‍👧‍👦", "🇳🇴", "1️⃣", "<:custom:123456789>"} {
		_, err := ParseEmoji(raw)
		require.NoError(t, err, raw)
	}
	got, err := ParseEmoji("<a:custom:123456789>")
	require.NoError(t, err)
	require.Equal(t, "custom:123456789", got)
}
func TestScoreAndRepostAfterSourceChange(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, "⭐")] = Votes{Positive: []snowflake.ID{1, 2, 3, 4, 5, 6}, Negative: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	require.Empty(t, f.published)
	f.votes[voteKey(100, "⭐")] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	e := entry(t, s, b)
	require.NotZero(t, e.CopyMessageID)
	f.votes[voteKey(e.CopyMessageID, "⭐")] = Votes{Positive: []snowflake.ID{1}, Negative: []snowflake.ID{7}}
	reconcile(t, s)
	e = entry(t, s, b)
	require.Zero(t, e.CopyMessageID)
	require.NotEmpty(t, e.WaitFingerprint)
	require.Len(t, f.deleted, 1)
	reconcile(t, s)
	require.Len(t, f.published, 1, "lost copy downvote must not immediately repost")
	// Restart retains the waiting fingerprint.
	s = New(s.db, f)
	reconcile(t, s)
	require.Len(t, f.published, 1)
	f.votes[voteKey(100, "⭐")] = Votes{Positive: []snowflake.ID{1, 2, 3, 4, 5}}
	reconcile(t, s)
	require.Len(t, f.published, 2)
}
func TestBoardsAndVotesAreIndependent(t *testing.T) {
	s, f, b := setup(t)
	second := model.Starboard{GuildID: 10, Name: "Hearts", ChannelID: 21, Emoji: "❤️", Threshold: 2, Enabled: true}
	require.NoError(t, s.SaveBoard(&second))
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	f.votes[voteKey(100, second.Emoji)] = Votes{Positive: []snowflake.ID{1, 2}}
	reconcile(t, s)
	a := entry(t, s, b)
	z := entry(t, s, second)
	require.Len(t, f.published, 2)
	f.votes[voteKey(a.CopyMessageID, b.Emoji)] = Votes{Negative: []snowflake.ID{9}}
	reconcile(t, s)
	require.Zero(t, entry(t, s, b).CopyMessageID)
	require.Equal(t, z.CopyMessageID, entry(t, s, second).CopyMessageID)
}
func TestModerationAndBlacklists(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	e := entry(t, s, b)
	require.NoError(t, s.Moderate(10, b.ID, "remove", 20, e.CopyMessageID))
	reconcile(t, s)
	require.True(t, entry(t, s, b).Suppressed)
	require.Len(t, f.published, 1)
	require.NoError(t, s.Moderate(10, b.ID, "restore", 30, 100))
	reconcile(t, s)
	require.Len(t, f.published, 2)
	require.NoError(t, s.Moderate(10, 0, "blacklist-channel", 30, 0))
	reconcile(t, s)
	require.Zero(t, entry(t, s, b).CopyMessageID)
	require.NoError(t, s.Moderate(10, 0, "unblacklist-channel", 30, 0))
	reconcile(t, s)
	require.Len(t, f.published, 3)
	require.Error(t, s.Moderate(11, b.ID, "remove", 30, 100))
}
func TestManualDeleteAndDisabledCleanup(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	e := entry(t, s, b)
	require.NoError(t, s.reconcile(10, 20, e.CopyMessageID, true, false))
	reconcile(t, s)
	require.True(t, entry(t, s, b).Suppressed)
	require.NoError(t, s.Moderate(10, b.ID, "restore", 30, 100))
	reconcile(t, s)
	require.NoError(t, s.SetEnabled(10, false))
	f.messages[100].Content = "edited"
	f.updated = nil
	reconcile(t, s)
	require.NotEmpty(t, f.updated)
	require.NoError(t, s.reconcile(10, 30, 100, true, false))
	require.Zero(t, entry(t, s, b).CopyMessageID)
	require.True(t, entry(t, s, b).SourceDeleted)
}
func TestAmbiguousPublishIsRecoveredNeverBlindlyRetried(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	f.failPublish = true
	require.Error(t, s.reconcile(10, 30, 100, false, true))
	require.NotEmpty(t, entry(t, s, b).SendNonce)
	s = New(s.db, f)
	require.ErrorIs(t, s.reconcile(10, 30, 100, false, true), ErrAmbiguousSend)
	require.Len(t, f.published, 1)
	f.recovered = 9999
	reconcile(t, s)
	require.Equal(t, snowflake.ID(9999), entry(t, s, b).CopyMessageID)
	require.Len(t, f.published, 1)
}
func TestForbiddenSourceIsNotDeletionAndCleanupRetries(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	f.failSource = true
	require.Error(t, s.reconcile(10, 30, 100, false, true))
	require.Empty(t, f.deleted)
	f.failSource = false
	f.failDelete = true
	require.NoError(t, s.DeleteBoard(10, b.ID))
	require.Error(t, s.reconcile(10, 30, 100, false, false))
	require.NotZero(t, entry(t, s, b).CopyMessageID)
	f.failDelete = false
	reconcile(t, s)
	require.NotEmpty(t, f.deleted)
}

func TestSettingsEditDoesNotCancelPendingDestinationMigration(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	old := entry(t, s, b).CopyMessageID
	b.ChannelID = 25
	require.NoError(t, s.SaveBoard(&b))
	b.Threshold = 3
	require.NoError(t, s.SaveBoard(&b))
	reconcile(t, s)
	require.Contains(t, f.deleted, old)
	require.Equal(t, snowflake.ID(25), entry(t, s, b).CopyChannelID)
	require.Len(t, f.published, 2)
}
func TestBlacklistCleansUpUnreadableOriginal(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	f.failSource = true
	require.NoError(t, s.Moderate(10, 0, "blacklist-channel", 30, 0))
	reconcile(t, s)
	require.Zero(t, entry(t, s, b).CopyMessageID)
}
func TestPendingAutomaticDeletionDoesNotSuppressEntry(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	e := entry(t, s, b)
	f.votes[voteKey(e.CopyMessageID, b.Emoji)] = Votes{Negative: []snowflake.ID{9}}
	f.failDelete = true
	require.Error(t, s.reconcile(10, 30, 100, false, true))
	// Discord deleted it despite the response error, so the next retry has no
	// copy votes. The stored cleanup intent must still finish its removal.
	f.failDelete = false
	delete(f.votes, voteKey(e.CopyMessageID, b.Emoji))
	reconcile(t, s)
	e = entry(t, s, b)
	require.Zero(t, e.CopyMessageID)
	require.False(t, e.Suppressed)
	require.Len(t, f.published, 1)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4, 5}}
	reconcile(t, s)
	require.Len(t, f.published, 2)
}

func TestEmojiRejectsNonEmojiSymbolsAndInvalidSequences(t *testing.T) {
	for _, raw := range []string{"⌘", "↤", "⭐‍⭐", "🏽", "🇦🇦", "<:emoji:123", "emoji:123>"} {
		_, err := ParseEmoji(raw)
		require.Error(t, err, raw)
	}
	_, err := ParseEmoji("🏴\U000e0067\U000e0062\U000e0073\U000e0063\U000e0074\U000e007f")
	require.NoError(t, err)
}

func TestKnownPublishedIDSurvivesReactionSeedingError(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	f.partialPublish = true
	require.Error(t, s.reconcile(10, 30, 100, false, true))
	e := entry(t, s, b)
	require.NotZero(t, e.CopyMessageID)
	require.Empty(t, e.SendNonce)
	f.partialPublish = false
	reconcile(t, s)
	require.Len(t, f.published, 1)
}
func TestDefiniteRejectionCanRetryWithoutAmbiguousRecovery(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	f.unsent = true
	require.ErrorIs(t, s.reconcile(10, 30, 100, false, true), ErrNotSent)
	require.Empty(t, entry(t, s, b).SendNonce)
	f.unsent = false
	reconcile(t, s)
	require.NotZero(t, entry(t, s, b).CopyMessageID)
}
func TestParentBlacklistCleansUpUnreadableThread(t *testing.T) {
	s, f, b := setup(t)
	f.parent = 99
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	f.failSource = true
	require.NoError(t, s.Moderate(10, 0, "blacklist-channel", 99, 0))
	reconcile(t, s)
	require.Zero(t, entry(t, s, b).CopyMessageID)
}
func TestMessageBlacklistCleansUpUnreadableTrackedOriginal(t *testing.T) {
	s, f, b := setup(t)
	f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
	reconcile(t, s)
	f.failSource = true
	require.NoError(t, s.Moderate(10, 0, "blacklist-message", 30, 100))
	reconcile(t, s)
	require.Zero(t, entry(t, s, b).CopyMessageID)
}

func TestExistingBoardCanBeDisabledAfterLosingDiscordPermissions(t *testing.T) {
	s, f, b := setup(t)
	f.validationError = errors.New("no permission")
	b.Enabled = false
	require.NoError(t, s.SaveBoard(&b))
	b.Enabled = true
	require.Error(t, s.SaveBoard(&b))
}

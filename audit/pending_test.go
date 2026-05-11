package audit

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/model"
)

// setupTestDB initialises a temporary SQLite-backed model.DB so audit.commit
// can write rows during pending-buffer tests. We can't unit-test the buffer
// in isolation without a DB because committing rows is the whole point.
func setupTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if _, err := model.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	// Tests share the package-level shouldLog cache; reset it so a prior
	// test's enabled-toggle value can't leak into one that runs against a
	// fresh DB with the same guild_id.
	shouldLogCacheMu.Lock()
	shouldLogCache = map[snowflake.ID]shouldLogCacheEntry{}
	shouldLogCacheMu.Unlock()
	t.Cleanup(func() { _ = os.Remove(dbPath) })
}

// guildEnabled persists a guild_settings row with audit logging on so
// shouldLog returns true. Without this, LogPending and Log are no-ops.
func guildEnabled(t *testing.T, guildID snowflake.ID) {
	t.Helper()
	settings, err := model.GetGuildSettings(guildID)
	require.NoError(t, err)
	settings.AuditLogEnabled = true
	require.NoError(t, model.SetGuildSettings(settings))
	// Match the production code path: web handler invalidates the cache
	// after a settings save so the new toggle takes effect immediately.
	InvalidateShouldLogCache(guildID)
}

func countRows(t *testing.T, guildID snowflake.ID) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.DB.Model(&model.AuditLogEntry{}).
		Where("guild_id = ?", guildID).Count(&count).Error)
	return count
}

func TestLogPending_FlushesAfterTTL(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1001)
	guildEnabled(t, guildID)

	target := snowflake.ID(2002)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMessageDelete,
		Category:   CategoryMessage,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetMessage,
		Source:     SourceGateway,
	}, []EnrichField{EnrichActor, EnrichReason})

	// Pre-TTL: the row hasn't been written yet.
	assert.EqualValues(t, 0, countRows(t, guildID))

	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 1, countRows(t, guildID), "row should have been committed after TTL")
}

func TestTryEnrich_AttachesActorAndCommits(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1002)
	guildEnabled(t, guildID)

	target := snowflake.ID(3003)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventGuildBan,
		Category:   CategoryGuild,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Reason:     "spam",
	}, []EnrichField{EnrichActor})

	moderator := snowflake.ID(4004)
	enriched := TryEnrich(guildID, EventGuildBan, &target, &moderator, ActorUser, "modUser", "different reason", MatchFirst)
	assert.Equal(t, 1, enriched)

	// commit happened immediately, before the TTL would have fired.
	assert.EqualValues(t, 1, countRows(t, guildID))

	var row model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).First(&row).Error)
	require.NotNil(t, row.ActorID)
	assert.Equal(t, moderator, *row.ActorID)
	assert.Equal(t, "user", row.ActorKind)
	// Reason was NOT in the enrichable list, so the original is preserved.
	assert.Equal(t, "spam", row.Reason)
}

func TestTryEnrich_BulkMatchAll(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1003)
	guildEnabled(t, guildID)

	for _, id := range []snowflake.ID{5001, 5002, 5003} {
		t := id
		LogPending(Entry{
			GuildID:    guildID,
			EventType:  EventMessageDelete,
			Category:   CategoryMessage,
			ActorKind:  ActorUnknown,
			TargetID:   &t,
			TargetKind: TargetMessage,
			Source:     SourceGateway,
		}, []EnrichField{EnrichActor, EnrichReason})
	}

	moderator := snowflake.ID(6006)
	// nil targetID = wildcard match, MatchAll = sweep all pending.
	enriched := TryEnrich(guildID, EventMessageDelete, nil, &moderator, ActorUser, "modUser", "bulk delete", MatchAll)
	assert.Equal(t, 3, enriched)
	assert.EqualValues(t, 3, countRows(t, guildID))
}

func TestCancelPending_DoesNotCommit(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1004)
	guildEnabled(t, guildID)

	target := snowflake.ID(7007)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventGuildBan,
		Category:   CategoryGuild,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
	}, []EnrichField{EnrichActor})

	cancelled := CancelPending(guildID, EventGuildBan, &target)
	assert.Equal(t, 1, cancelled)

	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 0, countRows(t, guildID), "cancelled entry must not be committed")
}

func TestFlushPending_CommitsAllInFlight(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1005)
	guildEnabled(t, guildID)

	for i := range 5 {
		t := snowflake.ID(8000 + i)
		LogPending(Entry{
			GuildID:    guildID,
			EventType:  EventMessageDelete,
			Category:   CategoryMessage,
			ActorKind:  ActorUnknown,
			TargetID:   &t,
			TargetKind: TargetMessage,
			Source:     SourceGateway,
		}, []EnrichField{EnrichActor})
	}
	FlushPending()
	assert.EqualValues(t, 5, countRows(t, guildID))
}

func TestEnrichWhitelist_DoesNotClobberNonWhitelistedFields(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1006)
	guildEnabled(t, guildID)

	target := snowflake.ID(9009)
	originalActor := snowflake.ID(1010)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventGuildBan,
		Category:   CategoryGuild,
		ActorID:    &originalActor,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Reason:     "original reason — never overwrite",
	}, []EnrichField{}) // empty whitelist — nothing is enrichable

	enricher := snowflake.ID(2020)
	TryEnrich(guildID, EventGuildBan, &target, &enricher, ActorUser, "modUser", "different reason", MatchFirst)

	var row model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).First(&row).Error)
	require.NotNil(t, row.ActorID)
	assert.Equal(t, originalActor, *row.ActorID, "actor must not be overwritten when not whitelisted")
	assert.Equal(t, "original reason — never overwrite", row.Reason)
}

// Native-arrives-first race: TryEnrich is called before LogPending lands
// the matching pending entry. Without the bidirectional buffer the
// gateway entry would commit unenriched after TTL.
func TestTryEnrich_NativeBeforeGateway(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1100)
	guildEnabled(t, guildID)

	target := snowflake.ID(11001)
	moderator := snowflake.ID(22002)

	// Native enrichment arrives first.
	enriched := TryEnrich(guildID, EventMemberUpdate, &target, &moderator, ActorUser, "modUser", "timeout", MatchFirst)
	assert.Equal(t, 0, enriched, "no pending entries yet — TryEnrich returns 0")

	// Gateway entry arrives a moment later — should pick up the buffered
	// enrichment and commit immediately.
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMemberUpdate,
		Category:   CategoryMember,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
	}, []EnrichField{EnrichActor, EnrichReason})

	// commit happened synchronously inside LogPending — no need to wait for TTL.
	assert.EqualValues(t, 1, countRows(t, guildID))

	var row model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).First(&row).Error)
	require.NotNil(t, row.ActorID)
	assert.Equal(t, moderator, *row.ActorID, "actor must be enriched even when native arrived first")
	assert.Equal(t, "timeout", row.Reason)
}

// Sanity: shouldLog returns false when the toggle is off.
func TestLog_NoopWhenDisabled(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1007)

	// Don't call guildEnabled — toggle stays at default false.
	var hits atomic.Int64
	actor := snowflake.ID(3030)
	target := snowflake.ID(3031)
	for range 3 {
		Log(Entry{
			GuildID:    guildID,
			EventType:  EventGuildBan,
			Category:   CategoryGuild,
			ActorID:    &actor,
			ActorKind:  ActorUser,
			TargetID:   &target,
			TargetKind: TargetUser,
			Source:     SourceGateway,
		})
		hits.Add(1)
	}
	assert.Equal(t, int64(3), hits.Load())
	assert.EqualValues(t, 0, countRows(t, guildID), "no rows should be written while disabled")
}

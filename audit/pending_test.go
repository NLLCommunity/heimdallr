package audit

import (
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/model"
)

// TestMain shrinks pendingTTL so the suite's TTL-driven sleeps run in
// tens of ms instead of seconds; without this, this file alone burns ~12s.
func TestMain(m *testing.M) {
	pendingTTL = 25 * time.Millisecond
	os.Exit(m.Run())
}

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
	enriched := TryEnrich(guildID, EventGuildBan, &target, nil, &moderator, ActorUser, "modUser", "different reason", MatchFirst, 0)
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
	// nil targetID = wildcard target match, MatchAll = sweep all pending.
	enriched := TryEnrich(guildID, EventMessageDelete, nil, nil, &moderator, ActorUser, "modUser", "bulk delete", MatchAll, 0)
	assert.Equal(t, 3, enriched)
	assert.EqualValues(t, 3, countRows(t, guildID))
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
	TryEnrich(guildID, EventGuildBan, &target, nil, &enricher, ActorUser, "modUser", "different reason", MatchFirst, 0)

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
	enriched := TryEnrich(guildID, EventMemberUpdate, &target, nil, &moderator, ActorUser, "modUser", "timeout", MatchFirst, 0)
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

// TryEnrichWithFallback's cold-cache path: native enrichment arrives,
// no pending entry is ever filed (the gateway listener bailed because
// the member wasn't cached), TTL expires — the fallback Entry must be
// committed so the moderator-driven event isn't silently lost.
func TestTryEnrichWithFallback_CommitsOnTTLExpiry(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1300)
	guildEnabled(t, guildID)

	target := snowflake.ID(13001)
	moderator := snowflake.ID(13002)

	fallback := Entry{
		GuildID:    guildID,
		EventType:  EventMemberNickChange,
		Category:   CategoryMember,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Reason:     "policy",
		Details: map[string]any{
			"nick_before":     "old",
			"nick_after":      "new",
			"actor_username":  "modUser",
			"target_username": "targetUser",
		},
	}

	enriched := TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "policy", MatchFirst, 0, fallback)
	assert.Equal(t, 0, enriched, "no pending entries — buffered with fallback")

	// Pre-TTL: nothing committed yet.
	assert.EqualValues(t, 0, countRows(t, guildID))

	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 1, countRows(t, guildID), "fallback should commit on TTL expiry")

	var row model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).First(&row).Error)
	require.NotNil(t, row.ActorID)
	assert.Equal(t, moderator, *row.ActorID)
	assert.Equal(t, "policy", row.Reason)
	assert.Contains(t, row.Details, "nick_after")
}

// TryEnrichWithFallback's warm-cache path: the gateway pending entry
// arrives within the TTL window, the buffered enrichment is consumed
// inline, the fallback must NOT also commit (would produce a duplicate).
func TestTryEnrichWithFallback_GatewayWinsNoDuplicate(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1301)
	guildEnabled(t, guildID)

	target := snowflake.ID(13011)
	moderator := snowflake.ID(13012)

	fallback := Entry{
		GuildID:    guildID,
		EventType:  EventMemberNickChange,
		Category:   CategoryMember,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Reason:     "policy",
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}

	// Native arrives first, buffered with fallback.
	TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "policy", MatchFirst, 0, fallback)

	// Gateway arrives within TTL → consumes the buffered enrichment.
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMemberNickChange,
		Category:   CategoryMember,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}, []EnrichField{EnrichActor, EnrichReason})

	// Gateway entry committed immediately on enrichment.
	assert.EqualValues(t, 1, countRows(t, guildID))

	// Past TTL: the enrichment's timer fires, but since the buffer entry
	// was already consumed by LogPending the fallback path must skip.
	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 1, countRows(t, guildID), "fallback must not duplicate after gateway already enriched")
}

// Disabled guilds must not receive backdoor rows via the fallback path.
// The native enrichment listener calls TryEnrichWithFallback for every
// moderator member-action regardless of the per-guild toggle; the
// shouldLog re-check inside expireEnrichment must drop the fallback
// when the guild has audit logging off.
func TestTryEnrichWithFallback_DisabledGuildDropsFallback(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1302)
	// Deliberately do NOT call guildEnabled — the guild_settings row
	// stays at AuditLogEnabled=false (default).

	target := snowflake.ID(13021)
	moderator := snowflake.ID(13022)

	fallback := Entry{
		GuildID:    guildID,
		Category:   CategoryMember,
		EventType:  EventMemberNickChange,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}

	TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "", MatchFirst, 0, fallback)

	time.Sleep(pendingTTL + 200*time.Millisecond)

	assert.EqualValues(t, 0, countRows(t, guildID),
		"disabled guild must not get fallback row written")
}

// The fallback's Category must persist to the row — without it, retention
// pruning (which filters on exact category match) silently leaks
// fallback rows forever, and the viewer's category filter misses them.
func TestTryEnrichWithFallback_PersistsCategory(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1303)
	guildEnabled(t, guildID)

	target := snowflake.ID(13031)
	moderator := snowflake.ID(13032)

	fallback := Entry{
		GuildID:    guildID,
		Category:   CategoryMember,
		EventType:  EventMemberNickChange,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}

	TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "", MatchFirst, 0, fallback)
	time.Sleep(pendingTTL + 200*time.Millisecond)

	var row model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).First(&row).Error)
	assert.Equal(t, string(CategoryMember), row.Category,
		"fallback row must carry its Category — retention pruning and viewer filters depend on it")
}

// FlushPending must commit unconsumed fallback rows on shutdown — the
// cold-cache event would otherwise be silently dropped if the bot exits
// before pendingTTL fires.
func TestTryEnrichWithFallback_FlushCommitsFallback(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1305)
	guildEnabled(t, guildID)

	target := snowflake.ID(13051)
	moderator := snowflake.ID(13052)

	fallback := Entry{
		GuildID:    guildID,
		Category:   CategoryMember,
		EventType:  EventMemberNickChange,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}

	TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "", MatchFirst, 0, fallback)

	// Pre-TTL: nothing committed yet — the enrichment is sitting in the
	// buffer waiting for a gateway entry that will never come.
	assert.EqualValues(t, 0, countRows(t, guildID))

	// Bot shutdown mid-window.
	FlushPending()

	assert.EqualValues(t, 1, countRows(t, guildID),
		"FlushPending must commit unconsumed fallback so cold-cache event isn't lost on shutdown")
}

// FlushPending must NOT commit a fallback whose enrichment was already
// consumed by an inline gateway entry — that row was committed by the
// gateway path and committing the fallback too would duplicate.
func TestTryEnrichWithFallback_FlushSkipsConsumedFallback(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1306)
	guildEnabled(t, guildID)

	target := snowflake.ID(13061)
	moderator := snowflake.ID(13062)

	fallback := Entry{
		GuildID:    guildID,
		Category:   CategoryMember,
		EventType:  EventMemberNickChange,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}

	// Native arrives first → buffered with fallback.
	TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "", MatchFirst, 0, fallback)
	// Gateway arrives → consumes the enrichment, commits the row.
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMemberNickChange,
		Category:   CategoryMember,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}, []EnrichField{EnrichActor, EnrichReason})

	assert.EqualValues(t, 1, countRows(t, guildID))

	// Now shutdown. The buffered enrichment was already removed by
	// LogPending, but the timer may still be alive — FlushPending must
	// not double-commit.
	FlushPending()

	assert.EqualValues(t, 1, countRows(t, guildID),
		"FlushPending must not duplicate a fallback whose enrichment was already consumed")
}

// Exercises the race between expireEnrichment and FlushPending: if
// expireEnrichment has set en.expired = true but FlushPending swaps the
// buffer before expireEnrichment can re-acquire buffer.mu, expireEnrichment
// sees stillInBuffer = false and skips. Without the fallbackCommitted
// check-and-set, FlushPending would also see expired = true and skip,
// silently dropping the row. The check-and-set guarantees exactly one
// path commits.
//
// Reproduces the race deterministically by setting en.expired manually
// before calling FlushPending (simulating expireEnrichment having
// claimed but not yet committed).
func TestTryEnrichWithFallback_ExpireFlushRace(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1307)
	guildEnabled(t, guildID)

	target := snowflake.ID(13071)
	moderator := snowflake.ID(13072)

	fallback := Entry{
		GuildID:    guildID,
		Category:   CategoryMember,
		EventType:  EventMemberNickChange,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
		Details:    map[string]any{"nick_before": "old", "nick_after": "new"},
	}

	TryEnrichWithFallback(guildID, EventMemberNickChange, &target, nil,
		&moderator, ActorUser, "modUser", "", MatchFirst, 0, fallback)

	// Locate the buffered enrichment and simulate expireEnrichment's
	// partial progress: en.expired set, but the buffer still contains
	// the entry. (Real expireEnrichment would release en.mu here and
	// race for buffer.mu against FlushPending.)
	pendingBuffer.mu.Lock()
	var en *pendingEnrichment
	for _, list := range pendingBuffer.enrichments {
		if len(list) > 0 {
			en = list[0]
			break
		}
	}
	pendingBuffer.mu.Unlock()
	require.NotNil(t, en, "enrichment should have been buffered")

	en.mu.Lock()
	en.expired = true
	en.mu.Unlock()

	// Now run FlushPending. With the buggy "skip if expired" check it
	// would observe en.expired == true and skip; with the fallbackCommitted
	// check-and-set it commits the fallback because nobody else has yet.
	FlushPending()

	// Give any in-flight timer callback a moment to run; it should bail
	// on en.expired without writing anything (and FlushPending stopped
	// the timer anyway).
	time.Sleep(pendingTTL + 100*time.Millisecond)

	assert.EqualValues(t, 1, countRows(t, guildID),
		"FlushPending must commit fallback even when expireEnrichment claimed it but didn't get to commit")
}

// MatchAll + fallback is unsupported (would duplicate). The pairing must
// drop the fallback rather than silently emit two rows.
func TestTryEnrichWithFallback_MatchAllDropsFallback(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1304)
	guildEnabled(t, guildID)

	target := snowflake.ID(13041)
	moderator := snowflake.ID(13042)

	fallback := Entry{
		GuildID:    guildID,
		Category:   CategoryMessage,
		EventType:  EventMessageDelete,
		ActorID:    &moderator,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
	}

	TryEnrichWithFallback(guildID, EventMessageDelete, &target, nil,
		&moderator, ActorUser, "modUser", "", MatchAll, 0, fallback)
	time.Sleep(pendingTTL + 200*time.Millisecond)

	assert.EqualValues(t, 0, countRows(t, guildID),
		"MatchAll + fallback must drop the fallback (would otherwise duplicate)")
}

// MatchFirst enrichments are one-shot: once a matching gateway entry has
// been enriched, the enrichment must NOT linger in the buffer to latch
// onto the next unrelated gateway event for the same (guild, event,
// target). Concretely: a moderator changes a member's nick, then within
// the TTL window that same member changes their own nick. The second
// row must not be attributed to the moderator.
func TestTryEnrich_MatchFirstDoesNotBleedIntoNextEntry(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1400)
	guildEnabled(t, guildID)

	target := snowflake.ID(14001)
	moderator := snowflake.ID(14002)

	// 1st action: gateway arrives, native enrichment fills in the actor.
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMemberNickChange,
		ActorKind:  ActorUnknown,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
	}, []EnrichField{EnrichActor, EnrichReason})

	enriched := TryEnrich(guildID, EventMemberNickChange, &target, nil, &moderator, ActorUser, "modUser", "policy", MatchFirst, 0)
	assert.Equal(t, 1, enriched, "first gateway event should be enriched")

	// 2nd action — same target, same event type — fires within the TTL
	// window. It has no associated native audit entry (the user changed
	// their own nick), so it must commit unenriched.
	selfActor := target
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMemberNickChange,
		ActorID:    &selfActor,
		ActorKind:  ActorUser,
		TargetID:   &target,
		TargetKind: TargetUser,
		Source:     SourceGateway,
	}, []EnrichField{EnrichActor, EnrichReason})

	// Wait past TTL so the second pending entry flushes.
	time.Sleep(pendingTTL + 200*time.Millisecond)

	assert.EqualValues(t, 2, countRows(t, guildID))

	var rows []model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).Order("created_at ASC").Find(&rows).Error)
	require.Len(t, rows, 2)

	require.NotNil(t, rows[0].ActorID)
	assert.Equal(t, moderator, *rows[0].ActorID, "first row attributed to moderator")
	assert.Equal(t, "policy", rows[0].Reason)

	require.NotNil(t, rows[1].ActorID)
	assert.Equal(t, target, *rows[1].ActorID, "second row must NOT be attributed to moderator — it's a self-action")
	assert.Equal(t, "", rows[1].Reason, "second row must NOT inherit moderator's reason")
}

// Regression: with the prior wildcard MatchAll for message-delete, an
// unrelated user-self-delete in another channel would be swept into a
// moderator's attribution if it landed within the TTL window. The
// (channel_id, author_id) required-details match must scope enrichment
// to entries that actually correspond to the native audit log row.
func TestTryEnrich_MessageDelete_DetailsScope(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1500)
	guildEnabled(t, guildID)

	modChannel := snowflake.ID(7100)
	otherChannel := snowflake.ID(7200)
	victim := snowflake.ID(7300)
	bystander := snowflake.ID(7301)

	// Mod-deleted: in modChannel, authored by victim.
	modMsg := snowflake.ID(7501)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMessageDelete,
		Category:   CategoryMessage,
		ActorID:    &victim,
		ActorKind:  ActorUser,
		TargetID:   &modMsg,
		TargetKind: TargetMessage,
		Source:     SourceGateway,
		Details: map[string]any{
			"channel_id":      modChannel.String(),
			"author_id":       victim.String(),
			"author_username": "victim",
			"actor_username":  "victim",
		},
	}, []EnrichField{EnrichActor, EnrichReason})

	// Unrelated self-delete: different channel, different author.
	selfMsg := snowflake.ID(7502)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMessageDelete,
		Category:   CategoryMessage,
		ActorID:    &bystander,
		ActorKind:  ActorUser,
		TargetID:   &selfMsg,
		TargetKind: TargetMessage,
		Source:     SourceGateway,
		Details: map[string]any{
			"channel_id":      otherChannel.String(),
			"author_id":       bystander.String(),
			"author_username": "bystander",
			"actor_username":  "bystander",
		},
	}, []EnrichField{EnrichActor, EnrichReason})

	moderator := snowflake.ID(7400)
	// Native audit log fired for the mod's single delete in modChannel:
	// TargetID = victim, Options.ChannelID = modChannel.
	required := map[string]string{
		"channel_id": modChannel.String(),
		"author_id":  victim.String(),
	}
	enriched := TryEnrich(guildID, EventMessageDelete, nil, required,
		&moderator, ActorUser, "modUser", "spam", MatchFirst, 0)
	assert.Equal(t, 1, enriched, "only the mod-deleted message should be enriched")

	// Let the unrelated self-delete flush unenriched.
	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 2, countRows(t, guildID))

	var rows []model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).Order("target_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)

	// rows[0] = lowest target_id = modMsg; rows[1] = selfMsg.
	require.NotNil(t, rows[0].ActorID)
	assert.Equal(t, moderator, *rows[0].ActorID, "mod's delete attributed to moderator")
	assert.Equal(t, "spam", rows[0].Reason)

	require.NotNil(t, rows[1].ActorID)
	assert.Equal(t, bystander, *rows[1].ActorID, "self-delete in another channel must NOT inherit mod attribution")
	assert.Equal(t, "", rows[1].Reason)
}

// Native-arrives-first variant of the misattribution scenario: the
// buffered enrichment must apply only to pending entries whose Details
// satisfy the requiredDetails predicate, not to any same-key entry that
// happens to land within TTL.
func TestTryEnrich_MessageDelete_NativeFirst_DetailsScope(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1501)
	guildEnabled(t, guildID)

	modChannel := snowflake.ID(7600)
	otherChannel := snowflake.ID(7700)
	victim := snowflake.ID(7800)
	bystander := snowflake.ID(7801)
	moderator := snowflake.ID(7900)

	// Native enrichment lands first — nothing pending yet.
	required := map[string]string{
		"channel_id": modChannel.String(),
		"author_id":  victim.String(),
	}
	enriched := TryEnrich(guildID, EventMessageDelete, nil, required,
		&moderator, ActorUser, "modUser", "spam", MatchFirst, 0)
	assert.Equal(t, 0, enriched, "no pending yet — buffered for later")

	// Bystander's unrelated self-delete in another channel arrives next.
	// Must not pick up the buffered enrichment.
	bystanderMsg := snowflake.ID(7901)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMessageDelete,
		Category:   CategoryMessage,
		ActorID:    &bystander,
		ActorKind:  ActorUser,
		TargetID:   &bystanderMsg,
		TargetKind: TargetMessage,
		Source:     SourceGateway,
		Details: map[string]any{
			"channel_id": otherChannel.String(),
			"author_id":  bystander.String(),
		},
	}, []EnrichField{EnrichActor, EnrichReason})

	// Victim's actually-mod-deleted message arrives — should pick up
	// the buffered enrichment and commit immediately with mod attribution.
	modMsg := snowflake.ID(7902)
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMessageDelete,
		Category:   CategoryMessage,
		ActorID:    &victim,
		ActorKind:  ActorUser,
		TargetID:   &modMsg,
		TargetKind: TargetMessage,
		Source:     SourceGateway,
		Details: map[string]any{
			"channel_id": modChannel.String(),
			"author_id":  victim.String(),
		},
	}, []EnrichField{EnrichActor, EnrichReason})

	// Let the bystander entry flush unenriched.
	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 2, countRows(t, guildID))

	var rows []model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).Order("target_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)

	// rows[0] = lowest target_id = bystanderMsg, rows[1] = modMsg.
	require.NotNil(t, rows[0].ActorID)
	assert.Equal(t, bystander, *rows[0].ActorID, "bystander's self-delete must not inherit mod attribution")
	assert.Equal(t, "", rows[0].Reason)

	require.NotNil(t, rows[1].ActorID)
	assert.Equal(t, moderator, *rows[1].ActorID, "victim's mod-deleted message attributed to moderator")
	assert.Equal(t, "spam", rows[1].Reason)
}

// max parameter caps how many gateway entries a MatchAll buffered
// enrichment may claim. With max=2 and three matching pendings arriving
// after the native, only the first two get mod attribution; the third
// must commit unenriched. This prevents a Discord-aggregated burst
// (Options.Count=N) from latching onto an unrelated same-(channel,
// author) delete that happens to arrive within TTL once the burst is
// fully attributed.
func TestTryEnrich_MatchAll_CountCap(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1502)
	guildEnabled(t, guildID)

	channel := snowflake.ID(8100)
	victim := snowflake.ID(8200)
	moderator := snowflake.ID(8300)

	required := map[string]string{
		"channel_id": channel.String(),
		"author_id":  victim.String(),
	}
	// Native arrives first with Count=2.
	enriched := TryEnrich(guildID, EventMessageDelete, nil, required,
		&moderator, ActorUser, "modUser", "spam", MatchAll, 2)
	assert.Equal(t, 0, enriched)

	makePending := func(id snowflake.ID) {
		t.Helper()
		mid := id
		LogPending(Entry{
			GuildID:    guildID,
			EventType:  EventMessageDelete,
			Category:   CategoryMessage,
			ActorID:    &victim,
			ActorKind:  ActorUser,
			TargetID:   &mid,
			TargetKind: TargetMessage,
			Source:     SourceGateway,
			Details: map[string]any{
				"channel_id": channel.String(),
				"author_id":  victim.String(),
			},
		}, []EnrichField{EnrichActor, EnrichReason})
	}

	// Three matching pendings — only the first two should be enriched.
	makePending(8401)
	makePending(8402)
	makePending(8403)

	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 3, countRows(t, guildID))

	var rows []model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).Order("target_id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)

	require.NotNil(t, rows[0].ActorID)
	assert.Equal(t, moderator, *rows[0].ActorID, "1st pending attributed to mod")
	require.NotNil(t, rows[1].ActorID)
	assert.Equal(t, moderator, *rows[1].ActorID, "2nd pending attributed to mod (cap reached)")
	require.NotNil(t, rows[2].ActorID)
	assert.Equal(t, victim, *rows[2].ActorID, "3rd pending past the cap — must commit unenriched")
	assert.Equal(t, "", rows[2].Reason)
}

// Mixed-arrival count cap: some matching pendings already in buffer when
// the native arrives (so they get inline matched), with cap budget left
// over for late arrivals. Total mod-attributed entries must equal max.
func TestTryEnrich_MatchAll_CountCap_InlinePlusBuffered(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1503)
	guildEnabled(t, guildID)

	channel := snowflake.ID(8500)
	victim := snowflake.ID(8600)
	moderator := snowflake.ID(8700)

	makePending := func(id snowflake.ID) {
		t.Helper()
		mid := id
		LogPending(Entry{
			GuildID:    guildID,
			EventType:  EventMessageDelete,
			Category:   CategoryMessage,
			ActorID:    &victim,
			ActorKind:  ActorUser,
			TargetID:   &mid,
			TargetKind: TargetMessage,
			Source:     SourceGateway,
			Details: map[string]any{
				"channel_id": channel.String(),
				"author_id":  victim.String(),
			},
		}, []EnrichField{EnrichActor, EnrichReason})
	}

	// 2 pendings filed first — these will match inline.
	makePending(8801)
	makePending(8802)

	required := map[string]string{
		"channel_id": channel.String(),
		"author_id":  victim.String(),
	}
	// Native with max=3 arrives: inline-matches 2, buffers with remaining=1.
	enriched := TryEnrich(guildID, EventMessageDelete, nil, required,
		&moderator, ActorUser, "modUser", "spam", MatchAll, 3)
	assert.Equal(t, 2, enriched, "2 inline matches; 1 budget unit left in buffer")

	// 2 more pendings arrive after the native: the first picks up the
	// remaining budget, the second commits unenriched.
	makePending(8803)
	makePending(8804)

	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 4, countRows(t, guildID))

	var rows []model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).Order("target_id ASC").Find(&rows).Error)
	require.Len(t, rows, 4)

	for i, want := range []snowflake.ID{moderator, moderator, moderator, victim} {
		require.NotNil(t, rows[i].ActorID, "row %d has nil ActorID", i)
		assert.Equal(t, want, *rows[i].ActorID, "row %d actor", i)
	}
	assert.Equal(t, "spam", rows[0].Reason)
	assert.Equal(t, "spam", rows[1].Reason)
	assert.Equal(t, "spam", rows[2].Reason)
	assert.Equal(t, "", rows[3].Reason, "4th pending past the cap — must be unenriched")
}

// Cache-miss pendings (no author_id captured in Details) must NOT match
// an enrichment requiring author_id, even when channel_id matches. The
// pending commits unenriched after TTL — that's the no-misattribution
// guarantee the audit_message.go cache-miss branch relies on.
func TestTryEnrich_DetailsMatch_RequiresAllKeys(t *testing.T) {
	setupTestDB(t)
	guildID := snowflake.ID(1504)
	guildEnabled(t, guildID)

	channel := snowflake.ID(9100)
	victim := snowflake.ID(9200)
	moderator := snowflake.ID(9300)
	messageID := snowflake.ID(9400)

	// Pending without author_id — simulates cache-miss in OnAuditMessageDelete.
	LogPending(Entry{
		GuildID:    guildID,
		EventType:  EventMessageDelete,
		Category:   CategoryMessage,
		ActorKind:  ActorUnknown,
		TargetID:   &messageID,
		TargetKind: TargetMessage,
		Source:     SourceGateway,
		Details: map[string]any{
			"channel_id": channel.String(),
			// author_id deliberately absent
		},
	}, []EnrichField{EnrichActor, EnrichReason})

	// Native enrichment requires (channel_id, author_id). Channel matches,
	// author_id is missing from the pending — must not bind.
	required := map[string]string{
		"channel_id": channel.String(),
		"author_id":  victim.String(),
	}
	enriched := TryEnrich(guildID, EventMessageDelete, nil, required,
		&moderator, ActorUser, "modUser", "spam", MatchFirst, 0)
	assert.Equal(t, 0, enriched, "missing author_id must not match")

	time.Sleep(pendingTTL + 200*time.Millisecond)
	assert.EqualValues(t, 1, countRows(t, guildID))

	var row model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).First(&row).Error)
	assert.Nil(t, row.ActorID, "cache-miss pending must commit unenriched, not picked up by mod's enrichment")
	assert.Equal(t, "", row.Reason)
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

// Stress test: many goroutines mixing LogPending and TryEnrich for the
// same (guild, eventType) key under heavy target contention.
//
// What this catches:
//   - data races (under `-race`), including any unlocked access to the
//     pending buffer or per-entry state;
//   - panics from buffer state corruption (double-stop on timers, slice
//     index errors when consume-then-append paths interleave);
//   - intra-row field scrambling: each row's (actor, target, reason)
//     tuple is asserted to come from a single submission, so a partial
//     write that copied actor from submission A and reason from
//     submission B would surface.
//
// What this does NOT catch: whole-payload swaps where TryEnrich_A's
// (actor, reason) lands on LogPending_B's pending entry. Because
// enrichment copies the whole field group atomically per pending entry,
// such a swap would still produce a self-consistent (actor_A,
// target_B, reason_A) row — and the assertion's `submitted[row.Reason]`
// lookup would re-anchor on reason_A's submission, which has target_A
// not target_B, so the target mismatch WOULD actually trip. (Verified
// by inspection: the assertion does compare target.) So this test does
// catch the cross-target case after all, just not via the path the
// docstring originally described.
func TestPendingBuffer_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped under -short")
	}
	setupTestDB(t)
	guildID := snowflake.ID(2_000_001)
	guildEnabled(t, guildID)

	const (
		producers = 32
		perGoro   = 50
	)
	// Small target pool so producers contend for the same buffer keys
	// frequently — that's what creates the conditions for any logic
	// race to surface. Every (actor, reason) tuple is still unique per
	// iteration so cross-pairings would assert.
	targetPool := make([]snowflake.ID, 16)
	for i := range targetPool {
		targetPool[i] = snowflake.ID(3_000_000 + i)
	}

	type submission struct {
		target snowflake.ID
		actor  snowflake.ID
		reason string
	}
	var submittedMu sync.Mutex
	submitted := map[string]submission{} // reason → submission

	var wg sync.WaitGroup
	for g := 0; g < producers; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(gid) * 1_000_003))
			for i := 0; i < perGoro; i++ {
				target := targetPool[r.Intn(len(targetPool))]
				actor := snowflake.ID(5_000_000 + gid*perGoro + i)
				// Reason is the unique tag we use to pair commits to submissions.
				reason := "stress-" + strconv.Itoa(gid) + "-" + strconv.Itoa(i)

				submittedMu.Lock()
				submitted[reason] = submission{target: target, actor: actor, reason: reason}
				submittedMu.Unlock()

				if r.Intn(2) == 0 {
					// Gateway-first: file pending, then native enrichment.
					t2 := target
					LogPending(Entry{
						GuildID:    guildID,
						EventType:  EventGuildBan,
						Category:   CategoryGuild,
						ActorKind:  ActorUnknown,
						TargetID:   &t2,
						TargetKind: TargetUser,
						Source:     SourceGateway,
					}, []EnrichField{EnrichActor, EnrichReason})
					// Small jitter so native sometimes wins the race.
					if r.Intn(2) == 0 {
						runtime.Gosched()
					}
					a := actor
					TryEnrich(guildID, EventGuildBan, &t2, nil, &a, ActorUser, "stress-actor", reason, MatchFirst, 0)
				} else {
					// Native-first: enrichment buffered, then gateway lands.
					a := actor
					t2 := target
					TryEnrich(guildID, EventGuildBan, &t2, nil, &a, ActorUser, "stress-actor", reason, MatchFirst, 0)
					if r.Intn(2) == 0 {
						runtime.Gosched()
					}
					LogPending(Entry{
						GuildID:    guildID,
						EventType:  EventGuildBan,
						Category:   CategoryGuild,
						ActorKind:  ActorUnknown,
						TargetID:   &t2,
						TargetKind: TargetUser,
						Source:     SourceGateway,
					}, []EnrichField{EnrichActor, EnrichReason})
				}
			}
		}(g)
	}
	wg.Wait()

	// Wait past TTL so any orphaned pendings flush, then verify no rows
	// got crossed wires.
	time.Sleep(pendingTTL + 500*time.Millisecond)
	FlushPending() // belt-and-braces in case any entries are still inflight

	var rows []model.AuditLogEntry
	require.NoError(t, model.DB.Where("guild_id = ?", guildID).Find(&rows).Error)

	// Every row was created by a LogPending call, so the row count should
	// match the number of LogPending submissions. Each goroutine made one
	// LogPending per iteration.
	assert.Equal(t, producers*perGoro, len(rows), "every LogPending submission must produce exactly one row")

	matched := 0
	for _, row := range rows {
		sub, ok := submitted[row.Reason]
		if !ok {
			// Unenriched (race-lost) commits leave Reason as "". Skip.
			continue
		}
		matched++
		require.NotNil(t, row.ActorID, "row enriched with reason %q but missing actor", row.Reason)
		assert.Equal(t, sub.actor, *row.ActorID, "actor mismatch for reason %q (cross-contamination)", row.Reason)
		require.NotNil(t, row.TargetID)
		assert.Equal(t, sub.target, *row.TargetID, "target mismatch for reason %q", row.Reason)
	}
	// Sanity: with the buffer working, the vast majority of submissions
	// should successfully pair up. We don't insist on 100% because the
	// TTL race can in principle leave some unenriched; <50% would indicate
	// the buffer isn't doing its job.
	require.Greater(t, matched, producers*perGoro/2,
		"expected > half of pairs to enrich successfully, got %d / %d", matched, producers*perGoro)
}

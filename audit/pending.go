package audit

import (
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// pendingTTL is how long the pending, enrichment, and suppression buffers
// hold entries waiting for their counterpart. Discord's native audit log
// entries normally arrive within a few hundred ms of the corresponding
// gateway event in either direction; 1.5s gives generous slack for ordering
// jitter without making moderation actions feel laggy in the viewer.
const pendingTTL = 1500 * time.Millisecond

// EnrichField names the fields that TryEnrich is permitted to fill.
// LogPending callers pass these per-event to prevent enrichment from
// clobbering known-good gateway-sourced fields (e.g. ban reason, which
// gateway+REST always supplies more completely than native audit).
type EnrichField string

const (
	EnrichActor  EnrichField = "actor"
	EnrichReason EnrichField = "reason"
)

// MatchMode controls TryEnrich's selection of pending entries when matching.
type MatchMode int

const (
	MatchFirst MatchMode = iota
	MatchAll
)

// pendingKey identifies a (guild, event) pair. TargetID is matched
// separately so wildcard / bulk matches work.
type pendingKey struct {
	guildID   snowflake.ID
	eventType EventType
}

type pendingEntry struct {
	entry      Entry
	enrichable map[EnrichField]bool
	timer      *time.Timer
	committed  bool
	mu         sync.Mutex
}

// pendingEnrichment is a native-audit-log fact that's waiting for its
// matching gateway entry. Stored when TryEnrich is called before the
// gateway listener has filed its pending entry — fixes the race where
// Discord's audit log entry handler runs ahead of the gateway event
// handler.
//
// actorUsername lets the enrichment overwrite Details["actor_username"]
// when the actor itself is being replaced (e.g. self-delete row enriched
// to a moderator-delete) so the viewer doesn't show a stale name.
type pendingEnrichment struct {
	targetID      *snowflake.ID // nil = wildcard (bulk events)
	actorID       *snowflake.ID
	actorKind     ActorKind
	actorUsername string
	reason        string
	match         MatchMode
	timer         *time.Timer
	expired       bool
	mu            sync.Mutex
}

// suppression marks a (guild, event, target) tuple as one whose next
// matching LogPending call must be silently dropped. Used by the kick
// path to handle the native-arrives-before-gateway race: cancelling
// already-buffered pending entries alone only catches the gateway-first
// case, since the leave hasn't yet been buffered when the native kick
// arrives in the other ordering.
type suppression struct {
	targetID snowflake.ID
	timer    *time.Timer
	consumed bool
	mu       sync.Mutex
}

// pendingBuffer holds all three race-handling maps under a single mutex
// so consume-then-append (in LogPending) and cancel-then-add-suppression
// (in Suppress) are mutually exclusive. A two-mutex design leaves a
// narrow window where LogPending can observe no suppression, then a
// concurrent Suppress can cancel-nothing-and-add-suppression, then
// LogPending appends a pending entry that the suppression should have
// caught — producing the duplicate row this whole machinery exists to
// prevent.
var pendingBuffer = struct {
	mu           sync.Mutex
	entries      map[pendingKey][]*pendingEntry
	enrichments  map[pendingKey][]*pendingEnrichment
	suppressions map[pendingKey][]*suppression
}{
	entries:      map[pendingKey][]*pendingEntry{},
	enrichments:  map[pendingKey][]*pendingEnrichment{},
	suppressions: map[pendingKey][]*suppression{},
}

// LogPending holds the entry briefly so a matching native audit log entry
// can attribute the actor / reason. Use this for gateway events with a
// possible Discord native audit log counterpart.
//
// Resolution order, all under a single pendingBuffer.mu critical section
// so concurrent producers can't interleave between the checks:
//  1. Matching suppression → drop the entry silently (kick race).
//  2. Matching enrichment → apply it and commit immediately.
//  3. Otherwise → register as pending; commit after pendingTTL.
func LogPending(entry Entry, enrichable []EnrichField) {
	if entry.Category == "" {
		entry.Category = EventCategory(entry.EventType)
	}
	if entry.Category == "" {
		return
	}
	if !shouldLog(entry.GuildID) {
		return
	}

	allowed := make(map[EnrichField]bool, len(enrichable))
	for _, f := range enrichable {
		allowed[f] = true
	}

	key := pendingKey{guildID: entry.GuildID, eventType: entry.EventType}

	pendingBuffer.mu.Lock()

	if s := findAndRemoveSuppressionLocked(key, entry.TargetID); s != nil {
		pendingBuffer.mu.Unlock()
		// Mark consumed and stop the TTL timer outside the buffer lock —
		// preserves the buffer.mu-then-inner-mu ordering convention.
		s.mu.Lock()
		s.consumed = true
		s.timer.Stop()
		s.mu.Unlock()
		return
	}

	matched := findAndRemoveEnrichmentLocked(key, entry.TargetID)

	var pe *pendingEntry
	if matched == nil {
		pe = &pendingEntry{
			entry:      entry,
			enrichable: allowed,
		}
		// Timer is created under the lock so its callback can find pe in
		// the buffer once we release. The callback acquires the same lock
		// and is therefore serialized against this section.
		pe.timer = time.AfterFunc(pendingTTL, func() {
			flushOne(pe)
		})
		pendingBuffer.entries[key] = append(pendingBuffer.entries[key], pe)
	}

	pendingBuffer.mu.Unlock()

	if matched != nil {
		matched.mu.Lock()
		applyEnrichment(&entry, allowed, matched)
		// For MatchFirst, stop the enrichment's timer so its expiry
		// callback doesn't try to clean up an already-removed slot.
		if matched.match != MatchAll {
			matched.expired = true
			matched.timer.Stop()
		}
		matched.mu.Unlock()
		commit(entry)
	}
}

// findAndRemoveSuppressionLocked returns the first matching suppression
// for key and target, removing it from the buffer. MUST be called with
// pendingBuffer.mu held.
func findAndRemoveSuppressionLocked(key pendingKey, target *snowflake.ID) *suppression {
	list := pendingBuffer.suppressions[key]
	if len(list) == 0 {
		return nil
	}
	for i, s := range list {
		if target != nil && s.targetID == *target {
			pendingBuffer.suppressions[key] = append(list[:i], list[i+1:]...)
			if len(pendingBuffer.suppressions[key]) == 0 {
				delete(pendingBuffer.suppressions, key)
			}
			return s
		}
	}
	return nil
}

// findAndRemoveEnrichmentLocked returns a matching enrichment for key and
// target, removing it from the buffer (except for sticky MatchAll
// enrichments, which stay until TTL). MUST be called with
// pendingBuffer.mu held.
func findAndRemoveEnrichmentLocked(key pendingKey, target *snowflake.ID) *pendingEnrichment {
	enrichments := pendingBuffer.enrichments[key]
	if len(enrichments) == 0 {
		return nil
	}
	var matched *pendingEnrichment
	var remaining []*pendingEnrichment
	for _, en := range enrichments {
		if matched == nil && enrichmentTargetMatches(en, target) {
			matched = en
			if en.match == MatchAll {
				remaining = append(remaining, en)
			}
			continue
		}
		remaining = append(remaining, en)
	}
	if len(remaining) == 0 {
		delete(pendingBuffer.enrichments, key)
	} else {
		pendingBuffer.enrichments[key] = remaining
	}
	return matched
}

// TryEnrich attaches actor / reason fields to pending entries that match
// (guildID, eventType, targetID), then commits them. If no matching
// pending entry exists, the enrichment is buffered for pendingTTL so a
// gateway entry arriving slightly later can still pick it up.
//
// actorUsername should be the actor's resolved Username (typically from
// the disgo member cache); when non-empty and EnrichActor is whitelisted,
// it overwrites Details["actor_username"] so the viewer doesn't show a
// stale name from a self-action that's now attributed to a moderator.
//
// match controls how many pending entries are enriched: MatchFirst (single)
// or MatchAll (every pending entry under the key — for bulk events).
// Returns the number of pending entries enriched immediately.
func TryEnrich(
	guildID snowflake.ID,
	eventType EventType,
	targetID *snowflake.ID,
	actorID *snowflake.ID,
	actorKind ActorKind,
	actorUsername string,
	reason string,
	match MatchMode,
) int {
	key := pendingKey{guildID: guildID, eventType: eventType}

	pendingBuffer.mu.Lock()
	pending := pendingBuffer.entries[key]
	var matched []*pendingEntry
	var remaining []*pendingEntry
	for _, pe := range pending {
		if matchesTarget(pe.entry.TargetID, targetID) && (match == MatchAll || len(matched) == 0) {
			matched = append(matched, pe)
		} else {
			remaining = append(remaining, pe)
		}
	}
	if len(remaining) == 0 {
		delete(pendingBuffer.entries, key)
	} else {
		pendingBuffer.entries[key] = remaining
	}

	// Always store the enrichment so any in-flight gateway entry that
	// hasn't yet hit LogPending can still pick it up. For MatchFirst, the
	// enrichment expires once consumed by LogPending; for MatchAll it's
	// sticky for the full TTL so every member of a bulk burst is covered.
	en := &pendingEnrichment{
		targetID:      copySnowflake(targetID),
		actorID:       copySnowflake(actorID),
		actorKind:     actorKind,
		actorUsername: actorUsername,
		reason:        reason,
		match:         match,
	}
	en.timer = time.AfterFunc(pendingTTL, func() {
		expireEnrichment(key, en)
	})
	pendingBuffer.enrichments[key] = append(pendingBuffer.enrichments[key], en)
	pendingBuffer.mu.Unlock()

	for _, pe := range matched {
		pe.mu.Lock()
		if pe.committed {
			pe.mu.Unlock()
			continue
		}
		if pe.enrichable[EnrichActor] && actorID != nil {
			pe.entry.ActorID = actorID
			pe.entry.ActorKind = actorKind
			if actorUsername != "" {
				setDetail(&pe.entry, "actor_username", actorUsername)
			}
		}
		if pe.enrichable[EnrichReason] && reason != "" {
			pe.entry.Reason = reason
		}
		pe.committed = true
		pe.timer.Stop()
		pe.mu.Unlock()
		commit(pe.entry)
	}
	return len(matched)
}

// setDetail mutates an Entry's Details map, allocating it on first write.
// Used by enrichment paths that need to overwrite a stale username when
// the actor is being replaced.
func setDetail(entry *Entry, key string, value any) {
	if entry.Details == nil {
		entry.Details = map[string]any{}
	}
	entry.Details[key] = value
}

// CancelPending removes a pending entry without committing. Returns the
// number of pending entries cancelled. Public for callers that need to
// drop an in-flight entry without setting up a TTL-bound suppression
// (e.g. a follow-up gateway event that obviates the pending one).
//
// For the kick race, prefer Suppress: CancelPending only catches the
// gateway-first ordering, not the native-first one.
func CancelPending(guildID snowflake.ID, eventType EventType, targetID *snowflake.ID) int {
	key := pendingKey{guildID: guildID, eventType: eventType}
	pendingBuffer.mu.Lock()
	cancelled := cancelMatchingPendingLocked(key, targetID)
	pendingBuffer.mu.Unlock()

	for _, pe := range cancelled {
		pe.mu.Lock()
		if !pe.committed {
			pe.committed = true
			pe.timer.Stop()
		}
		pe.mu.Unlock()
	}
	return len(cancelled)
}

// cancelMatchingPendingLocked removes pending entries matching the target
// from the buffer and returns them. MUST be called with pendingBuffer.mu
// held. Callers are responsible for marking the returned entries
// committed under their own mu (release the buffer lock first to keep
// buffer.mu-before-pe.mu ordering).
func cancelMatchingPendingLocked(key pendingKey, targetID *snowflake.ID) []*pendingEntry {
	pending := pendingBuffer.entries[key]
	if len(pending) == 0 {
		return nil
	}
	var cancelled []*pendingEntry
	var remaining []*pendingEntry
	for _, pe := range pending {
		if matchesTarget(pe.entry.TargetID, targetID) {
			cancelled = append(cancelled, pe)
		} else {
			remaining = append(remaining, pe)
		}
	}
	if len(remaining) == 0 {
		delete(pendingBuffer.entries, key)
	} else {
		pendingBuffer.entries[key] = remaining
	}
	return cancelled
}

// Suppress prevents the next matching LogPending call for
// (guildID, eventType, targetID) from buffering or committing an entry,
// AND cancels any pending entry already in the buffer. Used by the kick
// path so exactly one row (the kick) is recorded regardless of whether
// the gateway leave arrives before or after the native audit log entry.
//
// The cancel and the suppression-add happen atomically under
// pendingBuffer.mu so a concurrent LogPending cannot land its pending
// entry in between (otherwise the leave would commit unsuppressed).
//
// The suppression expires after pendingTTL if never consumed, so it
// can't accidentally swallow legitimate future leaves of the same user.
//
// targetID is required (no wildcard); a wildcard would silently drop the
// next leave of any user in the guild, which has no current use case and
// is a footgun.
func Suppress(guildID snowflake.ID, eventType EventType, targetID snowflake.ID) {
	key := pendingKey{guildID: guildID, eventType: eventType}

	s := &suppression{targetID: targetID}

	pendingBuffer.mu.Lock()
	cancelled := cancelMatchingPendingLocked(key, &targetID)
	s.timer = time.AfterFunc(pendingTTL, func() {
		expireSuppression(key, s)
	})
	pendingBuffer.suppressions[key] = append(pendingBuffer.suppressions[key], s)
	pendingBuffer.mu.Unlock()

	for _, pe := range cancelled {
		pe.mu.Lock()
		if !pe.committed {
			pe.committed = true
			pe.timer.Stop()
		}
		pe.mu.Unlock()
	}
}

// expireSuppression removes a suppression from the buffer when its TTL
// fires without being consumed. Idempotent — a concurrent consume is a
// no-op here.
func expireSuppression(key pendingKey, s *suppression) {
	s.mu.Lock()
	if s.consumed {
		s.mu.Unlock()
		return
	}
	s.consumed = true
	s.mu.Unlock()

	pendingBuffer.mu.Lock()
	list := pendingBuffer.suppressions[key]
	for i, item := range list {
		if item == s {
			pendingBuffer.suppressions[key] = append(list[:i], list[i+1:]...)
			if len(pendingBuffer.suppressions[key]) == 0 {
				delete(pendingBuffer.suppressions, key)
			}
			break
		}
	}
	pendingBuffer.mu.Unlock()
}

// FlushPending immediately commits every entry currently in the buffer
// and drops all unconsumed enrichments / suppressions. Wire this to bot
// shutdown so in-flight entries are not lost when the process exits
// before their TTL fires.
func FlushPending() {
	pendingBuffer.mu.Lock()
	allEntries := pendingBuffer.entries
	pendingBuffer.entries = map[pendingKey][]*pendingEntry{}
	allEnrichments := pendingBuffer.enrichments
	pendingBuffer.enrichments = map[pendingKey][]*pendingEnrichment{}
	allSuppressions := pendingBuffer.suppressions
	pendingBuffer.suppressions = map[pendingKey][]*suppression{}
	pendingBuffer.mu.Unlock()

	for _, pending := range allEntries {
		for _, pe := range pending {
			pe.mu.Lock()
			if pe.committed {
				pe.mu.Unlock()
				continue
			}
			pe.committed = true
			pe.timer.Stop()
			pe.mu.Unlock()
			commit(pe.entry)
		}
	}
	for _, list := range allEnrichments {
		for _, en := range list {
			en.mu.Lock()
			en.expired = true
			en.timer.Stop()
			en.mu.Unlock()
		}
	}
	for _, list := range allSuppressions {
		for _, s := range list {
			s.mu.Lock()
			s.consumed = true
			s.timer.Stop()
			s.mu.Unlock()
		}
	}
}

func applyEnrichment(entry *Entry, allowed map[EnrichField]bool, en *pendingEnrichment) {
	if allowed[EnrichActor] && en.actorID != nil {
		entry.ActorID = en.actorID
		entry.ActorKind = en.actorKind
		if en.actorUsername != "" {
			setDetail(entry, "actor_username", en.actorUsername)
		}
	}
	if allowed[EnrichReason] && en.reason != "" {
		entry.Reason = en.reason
	}
}

func flushOne(pe *pendingEntry) {
	pe.mu.Lock()
	if pe.committed {
		pe.mu.Unlock()
		return
	}
	pe.committed = true
	pe.mu.Unlock()

	// Best-effort removal from the buffer; if it's already been pulled by
	// TryEnrich we just commit (the committed flag prevents a double-write).
	key := pendingKey{guildID: pe.entry.GuildID, eventType: pe.entry.EventType}
	pendingBuffer.mu.Lock()
	pending := pendingBuffer.entries[key]
	for i, p := range pending {
		if p == pe {
			pendingBuffer.entries[key] = append(pending[:i], pending[i+1:]...)
			if len(pendingBuffer.entries[key]) == 0 {
				delete(pendingBuffer.entries, key)
			}
			break
		}
	}
	pendingBuffer.mu.Unlock()

	commit(pe.entry)
}

// expireEnrichment removes an unconsumed enrichment from the buffer when
// its TTL fires. No commit happens — an unmatched enrichment is just an
// audit log entry Discord told us about that we never saw via gateway,
// which we deliberately do not record (we only persist gateway-witnessed
// events to keep one row per real-world action).
func expireEnrichment(key pendingKey, en *pendingEnrichment) {
	en.mu.Lock()
	if en.expired {
		en.mu.Unlock()
		return
	}
	en.expired = true
	en.mu.Unlock()

	pendingBuffer.mu.Lock()
	list := pendingBuffer.enrichments[key]
	for i, e := range list {
		if e == en {
			pendingBuffer.enrichments[key] = append(list[:i], list[i+1:]...)
			if len(pendingBuffer.enrichments[key]) == 0 {
				delete(pendingBuffer.enrichments, key)
			}
			break
		}
	}
	pendingBuffer.mu.Unlock()
}

// matchesTarget returns true when filterTarget matches entryTarget. nil
// filterTarget acts as a wildcard so bulk-delete enrichment can sweep
// every pending message-delete in the guild.
func matchesTarget(entryTarget, filterTarget *snowflake.ID) bool {
	if filterTarget == nil {
		return true
	}
	if entryTarget == nil {
		return false
	}
	return *entryTarget == *filterTarget
}

// enrichmentTargetMatches is the LogPending-side counterpart to
// matchesTarget — checks whether a stored enrichment applies to a given
// pending entry's target.
func enrichmentTargetMatches(en *pendingEnrichment, entryTarget *snowflake.ID) bool {
	// Wildcard enrichment matches any entry, including ones with no target.
	if en.targetID == nil {
		return true
	}
	if entryTarget == nil {
		return false
	}
	return *en.targetID == *entryTarget
}

func copySnowflake(id *snowflake.ID) *snowflake.ID {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

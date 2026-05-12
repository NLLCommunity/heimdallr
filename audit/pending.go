// Pending-enrichment buffer for audit log entries that need moderator
// attribution from Discord's native audit log.
//
// Problem: gateway events (message delete, member ban, etc.) tell us
// WHAT happened, but not WHO did it when the actor is someone other
// than the affected user. The native audit log entries delivered via
// GuildAuditLogEntryCreate carry the moderator's identity, but they
// arrive on a different channel and can land either before or after
// the corresponding gateway event. We need to correlate the two so
// the persisted row carries both the event details and the actor.
//
// Design: a single buffer holds two sides of the race —
//   - pendingEntry: gateway-witnessed event waiting for native actor info
//   - pendingEnrichment: native actor info waiting for its gateway entry
//
// LogPending and TryEnrich each consult the buffer under one shared
// mutex (pendingBuffer.mu) and serialize the consume-vs-append decision,
// so the gateway-first and native-first orderings collapse to the same
// "look first, then add if no match" code path. Whichever arrives second
// commits the row inline; whichever arrives first sets a TTL timer and
// either commits unenriched when the timer fires (gateway-first miss) or
// drops silently (native-first miss — Discord-only events without a
// gateway counterpart aren't persisted).
//
// MatchFirst (one-shot) vs MatchAll (sticky for the burst) selects how
// many gateway entries a single native enrichment may attribute. MatchAll
// is capped by the count Discord reports in Options.Count so an aggregated
// burst can't accidentally claim unrelated same-key events arriving later
// within TTL. requiredDetails additionally narrows matching for
// message-delete, where TargetID alone (messageID gateway-side, authorID
// native-side) doesn't line up — the gateway entry stores
// (channel_id, author_id) in Details and the native side matches on those.
//
// Locking: see the invariant comment above pendingEntry below. The race
// detector catches data races; logic-race coverage lives in the
// *_test.go file.

package audit

import (
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// pendingTTL is how long the pending and enrichment buffers hold entries
// waiting for their counterpart. Discord's native audit log entries normally
// arrive within a few hundred ms of the corresponding gateway event in either
// direction; 1.5s gives generous slack for ordering jitter without making
// moderation actions feel laggy in the viewer.
//
// Declared as var (not const) so tests can shrink it via TestMain; production
// code never reassigns it.
var pendingTTL = 1500 * time.Millisecond

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

// pendingKey identifies a (guild, event) pair. TargetID and
// requiredDetails are matched separately so wildcard / bulk matches work.
type pendingKey struct {
	guildID   snowflake.ID
	eventType EventType
}

// lock ordering invariant: locks are never nested in either direction.
// Paths that begin with pendingBuffer.mu (LogPending, TryEnrich) release
// it before taking any pendingEntry.mu or pendingEnrichment.mu. Paths
// that begin with the per-entry / per-enrichment mu (flushOne,
// expireEnrichment) release it before re-acquiring pendingBuffer.mu.
// The findAndRemove* helpers run under pendingBuffer.mu only; callers
// do the per-X mutation after releasing it.

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
//
// requiredDetails further narrows matching when the natural TargetID
// doesn't line up between gateway and native sides (e.g. message-delete:
// pending TargetID is messageID, native TargetID is the author). Every
// (key, value) pair must be present and stringwise-equal in the pending
// entry's Details for the enrichment to apply.
//
// remaining caps how many gateway entries a MatchAll enrichment may
// enrich before it's removed from the buffer (independent of TTL):
//   - < 0  unlimited (sticky until TTL — used when the native side
//     doesn't carry a count, e.g. unknown-size bulk delete).
//   - > 0  decrement on each consumption; remove from buffer when zero
//     so a same-(channel, author) gateway event arriving after the
//     burst isn't misattributed to the moderator.
//
// MatchFirst ignores remaining (one-shot anyway). remaining is accessed
// only under pendingBuffer.mu, not en.mu.
type pendingEnrichment struct {
	targetID        *snowflake.ID // nil = wildcard target match
	requiredDetails map[string]string
	actorID         *snowflake.ID
	actorKind       ActorKind
	actorUsername   string
	reason          string
	match           MatchMode
	remaining       int
	timer           *time.Timer
	expired         bool
	mu              sync.Mutex
}

// pendingBuffer holds entries waiting for native-audit enrichment and
// enrichments waiting for their matching pending entry, under a single
// mutex so consume-then-append (in LogPending) and add (in TryEnrich) are
// serialized.
var pendingBuffer = struct {
	mu          sync.Mutex
	entries     map[pendingKey][]*pendingEntry
	enrichments map[pendingKey][]*pendingEnrichment
}{
	entries:     map[pendingKey][]*pendingEntry{},
	enrichments: map[pendingKey][]*pendingEnrichment{},
}

// LogPending holds the entry briefly so a matching native audit log entry
// can attribute the actor / reason. Use this for gateway events with a
// possible Discord native audit log counterpart.
//
// Resolution order, all under a single pendingBuffer.mu critical section
// so concurrent producers can't interleave between the checks:
//  1. Matching enrichment → apply it and commit immediately.
//  2. Otherwise → register as pending; commit after pendingTTL.
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

	matched := findAndRemoveEnrichmentLocked(key, &entry)

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

// findAndRemoveEnrichmentLocked returns a matching enrichment for the
// given entry, removing it from the buffer. MatchAll enrichments stay
// in the buffer until either (a) their TTL expires or (b) their
// remaining counter hits zero (set from Discord's Options.Count, which
// caps how many gateway entries an aggregated native row can attribute).
//
// First-match-wins: if multiple buffered enrichments could match the
// entry, only the earliest in iteration order is returned. Subsequent
// matches stay in the buffer for their own TTL — they'll bind to later
// pending entries or expire unconsumed. We don't try to merge match
// criteria; each native audit row corresponds to one TryEnrich call,
// and the surrounding callers don't produce overlapping criteria.
// MUST be called with pendingBuffer.mu held.
func findAndRemoveEnrichmentLocked(key pendingKey, entry *Entry) *pendingEnrichment {
	enrichments := pendingBuffer.enrichments[key]
	if len(enrichments) == 0 {
		return nil
	}
	var matched *pendingEnrichment
	var remaining []*pendingEnrichment
	for _, en := range enrichments {
		if matched == nil && enrichmentMatches(en, entry) {
			matched = en
			if en.match == MatchAll {
				// Decrement the consumption budget; > 0 = stay sticky,
				// 0 = exhausted (drop from buffer), < 0 = unlimited.
				if en.remaining > 0 {
					en.remaining--
				}
				if en.remaining != 0 {
					remaining = append(remaining, en)
				} else {
					// Exhausted — stop the TTL timer so it doesn't run
					// pointless work later. We deliberately don't set
					// en.expired here: we already hold buffer.mu and have
					// removed en from the list, so expireEnrichment's
					// fast path (which sets expired under en.mu) isn't
					// load-bearing — if the timer raced past Stop, it
					// will fail to find en in the list and exit cleanly.
					en.timer.Stop()
				}
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
// requiredDetails optionally narrows matching: when non-empty, the
// pending entry's Details must contain every (key, value) pair as a
// string. Used when targetID alone isn't a tight enough match — e.g.
// message-delete, where pending entries are keyed on messageID but
// Discord's native audit log reports (channel_id, author_id) and a
// wildcard sweep would misattribute concurrent unrelated deletes.
//
// actorUsername should be the actor's resolved Username (typically from
// the disgo member cache); when non-empty and EnrichActor is whitelisted,
// it overwrites Details["actor_username"] so the viewer doesn't show a
// stale name from a self-action that's now attributed to a moderator.
//
// match controls how many pending entries are enriched: MatchFirst (single)
// or MatchAll (every pending entry under the key — for bulk events).
//
// maxMatches caps how many gateway entries a MatchAll buffered enrichment
// may claim (inline matches count toward the cap, late arrivals consume
// the remainder). 0 or negative means unlimited (sticky until TTL); use
// when Discord doesn't supply a count. Ignored for MatchFirst.
//
// Returns the number of pending entries enriched immediately.
func TryEnrich(
	guildID snowflake.ID,
	eventType EventType,
	targetID *snowflake.ID,
	requiredDetails map[string]string,
	actorID *snowflake.ID,
	actorKind ActorKind,
	actorUsername string,
	reason string,
	match MatchMode,
	maxMatches int,
) int {
	// Clamp negative caps to "unlimited" so misbehaving callers can't
	// accidentally disable the buffered-enrichment path with -1.
	if maxMatches < 0 {
		maxMatches = 0
	}
	key := pendingKey{guildID: guildID, eventType: eventType}

	pendingBuffer.mu.Lock()
	pending := pendingBuffer.entries[key]
	var matched []*pendingEntry
	var remaining []*pendingEntry
	for _, pe := range pending {
		if matchesTarget(pe.entry.TargetID, targetID) &&
			detailsMatch(pe.entry.Details, requiredDetails) &&
			(match == MatchAll || len(matched) == 0) {
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

	// Decide whether to buffer a sticky enrichment for late-arriving
	// gateway entries. Buffer when:
	//   - nothing matched right now (native-first race — wait for the
	//     gateway entry to land within TTL); OR
	//   - MatchAll with budget left over after the inline sweep.
	//
	// MatchFirst with at least one inline match is fully attributed; a
	// lingering buffered enrichment would latch onto the next, unrelated
	// gateway event with the same key within pendingTTL.
	//
	// MatchAll with maxMatches > 0 caps the buffered enrichment so a
	// moderator's aggregated burst (e.g. AuditLogEventMessageDelete with
	// Count=N) can't silently attribute unrelated same-(channel, author)
	// deletes that happen to land within TTL after the burst is fully
	// consumed. -1 is the canonical "unlimited" sentinel stored on the
	// buffered enrichment.
	bufferEnrichment := false
	bufferRemaining := -1
	switch {
	case len(matched) == 0:
		// Native-first race: nothing pending yet. Buffer the full
		// budget; if MatchAll with a cap, store it on the enrichment.
		bufferEnrichment = true
		if match == MatchAll && maxMatches > 0 {
			bufferRemaining = maxMatches
		}
	case match == MatchAll && maxMatches == 0:
		// Unlimited sticky: caller didn't supply a count cap, keep the
		// enrichment alive for the TTL window.
		bufferEnrichment = true
	case match == MatchAll && maxMatches > len(matched):
		// Mixed: inline sweep consumed some, residual budget goes into
		// the buffer for late arrivals.
		bufferEnrichment = true
		bufferRemaining = maxMatches - len(matched)
	}
	if bufferEnrichment {
		en := &pendingEnrichment{
			targetID:        copySnowflake(targetID),
			requiredDetails: copyStringMap(requiredDetails),
			actorID:         copySnowflake(actorID),
			actorKind:       actorKind,
			actorUsername:   actorUsername,
			reason:          reason,
			match:           match,
			remaining:       bufferRemaining,
		}
		en.timer = time.AfterFunc(pendingTTL, func() {
			expireEnrichment(key, en)
		})
		pendingBuffer.enrichments[key] = append(pendingBuffer.enrichments[key], en)
	}
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

// FlushPending immediately commits every entry currently in the buffer
// and drops all unconsumed enrichments. Wire this to bot shutdown so
// in-flight entries are not lost when the process exits before their TTL
// fires.
func FlushPending() {
	pendingBuffer.mu.Lock()
	allEntries := pendingBuffer.entries
	pendingBuffer.entries = map[pendingKey][]*pendingEntry{}
	allEnrichments := pendingBuffer.enrichments
	pendingBuffer.enrichments = map[pendingKey][]*pendingEnrichment{}
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

// enrichmentMatches is the LogPending-side counterpart to the matching
// in TryEnrich — checks whether a stored enrichment applies to a given
// new pending entry. Combines target match (wildcard if en.targetID is
// nil) with the optional requiredDetails predicate.
func enrichmentMatches(en *pendingEnrichment, entry *Entry) bool {
	if en.targetID != nil {
		if entry.TargetID == nil || *en.targetID != *entry.TargetID {
			return false
		}
	}
	return detailsMatch(entry.Details, en.requiredDetails)
}

// detailsMatch returns true when every (key, value) in required is also
// present in have as a string-typed entry with the same value. Nil/empty
// required matches anything. Non-string values in have are treated as a
// miss — every required key currently passes string-stringified IDs.
func detailsMatch(have map[string]any, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}
	for k, want := range required {
		got, ok := have[k]
		if !ok {
			return false
		}
		s, ok := got.(string)
		if !ok || s != want {
			return false
		}
	}
	return true
}

func copySnowflake(id *snowflake.ID) *snowflake.ID {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

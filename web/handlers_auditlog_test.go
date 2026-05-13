package web

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePage(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 1},
		{"abc", 1},
		{"0", 1},
		{"-5", 1},
		{"1", 1},
		{"50", 50},
		{"10000", auditLogMaxPage},
		{"10001", auditLogMaxPage},
		{"999999999", auditLogMaxPage},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, parsePage(c.in))
		})
	}
}

func TestParseAuditLogFilters_DefaultLookbackApplies(t *testing.T) {
	// With no filters at all, From should default to today minus the
	// lookback window so a first-time visit isn't a full-history scan.
	r := httptest.NewRequest("GET", "/guild/123/auditlog", nil)
	f := parseAuditLogFilters(r)

	assert.Equal(t, "", f.Category)
	assert.Equal(t, "", f.EventType)
	assert.Equal(t, "", f.Actor)
	assert.Equal(t, "", f.Target)
	assert.Equal(t, "", f.To)

	parsed, err := time.Parse("2006-01-02", f.From)
	require.NoError(t, err, "default From should parse as YYYY-MM-DD")

	want := time.Now().Add(-auditLogDefaultLookback).UTC().Truncate(24 * time.Hour)
	// Allow a 1-day window in case the test crosses midnight UTC during run.
	delta := parsed.Sub(want)
	if delta < 0 {
		delta = -delta
	}
	assert.LessOrEqual(t, delta, 24*time.Hour, "default lookback should be ~%v ago, got %v", auditLogDefaultLookback, parsed)
}

func TestParseAuditLogFilters_NoDefaultWhenNarrowingFilterPresent(t *testing.T) {
	// A "narrowing" filter (actor/target/event/from/to) should suppress
	// the default From — the user already bounded the query.
	// Category alone splits the dataset only 3 ways, so it does NOT
	// suppress the default — a category-only query on a busy guild would
	// otherwise scan most of retention.
	cases := []string{
		"event_type=message.delete",
		"actor=alice",
		"target=%23general",
		"from=2025-01-01",
		"to=2025-12-31",
	}
	for _, qs := range cases {
		t.Run(qs, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/guild/123/auditlog?"+qs, nil)
			f := parseAuditLogFilters(r)
			// When the qs supplies From itself we want to see that exact
			// value, not the default. For all other cases From should be
			// empty (no default injected).
			if qs == "from=2025-01-01" {
				assert.Equal(t, "2025-01-01", f.From)
			} else {
				assert.Equal(t, "", f.From, "default From should NOT be injected when narrowing filter is present")
			}
		})
	}
}

func TestParseAuditLogFilters_CategoryAloneStillDefaultsLookback(t *testing.T) {
	// Category alone is too coarse to safely bound an unfiltered query,
	// so the 7-day default still applies.
	r := httptest.NewRequest("GET", "/guild/123/auditlog?category=member", nil)
	f := parseAuditLogFilters(r)
	assert.Equal(t, "member", f.Category)
	assert.NotEqual(t, "", f.From, "default From should still apply when only Category is set")
}

func TestParseAuditLogFilters_TrimsAndLowercases(t *testing.T) {
	// Whitespace around inputs gets trimmed; category is lowercased so
	// "?category=Message" matches the stored "message" constant.
	r := httptest.NewRequest("GET", "/guild/123/auditlog?category=Message&actor=%20alice%20&event_type=%20message.delete%20", nil)
	f := parseAuditLogFilters(r)
	assert.Equal(t, "message", f.Category)
	assert.Equal(t, "alice", f.Actor)
	assert.Equal(t, "message.delete", f.EventType)
}

// TestResolveActorQuery_NoCachePaths and TestResolveTargetQuery_NoCachePaths
// below exercise the paths through resolveActorQuery / resolveTargetQuery
// that don't touch client.Caches.Members or ChannelsForGuild. The cache-
// dipping paths (non-snowflake substring matching) need a bot.Client fake
// that this package doesn't have, so the snowflake fast-path, empty
// input, whitespace, and prefix-only inputs are what's covered here.
func TestResolveActorQuery_NoCachePaths(t *testing.T) {
	t.Run("empty input returns empty result", func(t *testing.T) {
		ids, text := resolveActorQuery(nil, 0, "")
		assert.Nil(t, ids)
		assert.Equal(t, "", text)
	})
	t.Run("whitespace-only returns empty result", func(t *testing.T) {
		ids, text := resolveActorQuery(nil, 0, "   ")
		assert.Nil(t, ids)
		assert.Equal(t, "", text)
	})
	t.Run("@-only returns empty result", func(t *testing.T) {
		ids, text := resolveActorQuery(nil, 0, " @ ")
		assert.Nil(t, ids)
		assert.Equal(t, "", text)
	})
	t.Run("bare snowflake returns exact id and no text fallback", func(t *testing.T) {
		ids, text := resolveActorQuery(nil, 0, "175928847299117063")
		require.Len(t, ids, 1)
		assert.Equal(t, snowflake.ID(175928847299117063), ids[0])
		assert.Equal(t, "", text, "snowflake match shouldn't fall back to LIKE")
	})
	t.Run("@-prefixed snowflake parses after prefix strip", func(t *testing.T) {
		ids, text := resolveActorQuery(nil, 0, "@175928847299117063")
		require.Len(t, ids, 1)
		assert.Equal(t, snowflake.ID(175928847299117063), ids[0])
		assert.Equal(t, "", text)
	})
	t.Run("whitespace around @-prefix is tolerated", func(t *testing.T) {
		ids, text := resolveActorQuery(nil, 0, "  @175928847299117063  ")
		require.Len(t, ids, 1)
		assert.Equal(t, snowflake.ID(175928847299117063), ids[0])
		assert.Equal(t, "", text)
	})
}

func TestResolveTargetQuery_NoCachePaths(t *testing.T) {
	t.Run("empty input returns empty result", func(t *testing.T) {
		ids, chIDs, text := resolveTargetQuery(nil, 0, "")
		assert.Nil(t, ids)
		assert.Nil(t, chIDs)
		assert.Equal(t, "", text)
	})
	t.Run("whitespace-only returns empty result", func(t *testing.T) {
		ids, chIDs, text := resolveTargetQuery(nil, 0, "   ")
		assert.Nil(t, ids)
		assert.Nil(t, chIDs)
		assert.Equal(t, "", text)
	})
	t.Run("#-only returns empty result", func(t *testing.T) {
		ids, chIDs, text := resolveTargetQuery(nil, 0, " # ")
		assert.Nil(t, ids)
		assert.Nil(t, chIDs)
		assert.Equal(t, "", text)
	})
	t.Run("bare snowflake matches both ids and channelIDs", func(t *testing.T) {
		// Bare snowflake — caller didn't disambiguate user vs channel, so
		// the filter matches against target_id and details.channel_id both.
		ids, chIDs, text := resolveTargetQuery(nil, 0, "175928847299117063")
		require.Len(t, ids, 1)
		require.Len(t, chIDs, 1)
		assert.Equal(t, snowflake.ID(175928847299117063), ids[0])
		assert.Equal(t, snowflake.ID(175928847299117063), chIDs[0])
		assert.Equal(t, "", text)
	})
	t.Run("#-prefixed snowflake also matches details.channel_id", func(t *testing.T) {
		// #-prefix says "this snowflake is a channel": match target_id
		// (channel-target events) AND details.channel_id (message events
		// that happened in the channel).
		ids, chIDs, text := resolveTargetQuery(nil, 0, "#175928847299117063")
		require.Len(t, ids, 1)
		require.Len(t, chIDs, 1)
		assert.Equal(t, snowflake.ID(175928847299117063), ids[0])
		assert.Equal(t, snowflake.ID(175928847299117063), chIDs[0])
		assert.Equal(t, "", text)
	})
	t.Run("@-prefixed snowflake skips details.channel_id", func(t *testing.T) {
		// @-prefix says "this snowflake is a user": match target_id only.
		// channelIDs must be empty so we don't also match message events
		// whose channel happens to share the user's snowflake.
		ids, chIDs, text := resolveTargetQuery(nil, 0, "@175928847299117063")
		require.Len(t, ids, 1)
		assert.Nil(t, chIDs, "@-scoped query must not populate channelIDs")
		assert.Equal(t, snowflake.ID(175928847299117063), ids[0])
		assert.Equal(t, "", text)
	})
}

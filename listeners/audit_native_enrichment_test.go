package listeners

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/assert"

	"github.com/NLLCommunity/heimdallr/audit"
)

func TestMemberUpdateEnrichmentTargets_TimeoutAddVsClear(t *testing.T) {
	cases := []struct {
		name    string
		changes []discord.AuditLogChange
		want    []audit.EventType
	}{
		{
			name: "timeout set produces timeout_add",
			changes: []discord.AuditLogChange{
				{Key: discord.AuditLogChangeKeyCommunicationDisabledUntil, NewValue: []byte(`"2025-01-01T00:00:00Z"`)},
			},
			want: []audit.EventType{audit.EventMemberTimeoutAdd},
		},
		{
			name: "timeout cleared (null new_value) produces timeout_clear",
			changes: []discord.AuditLogChange{
				{Key: discord.AuditLogChangeKeyCommunicationDisabledUntil, NewValue: []byte(`null`)},
			},
			want: []audit.EventType{audit.EventMemberTimeoutClear},
		},
		{
			name: "timeout cleared (empty new_value) produces timeout_clear",
			changes: []discord.AuditLogChange{
				{Key: discord.AuditLogChangeKeyCommunicationDisabledUntil, NewValue: nil},
			},
			want: []audit.EventType{audit.EventMemberTimeoutClear},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := memberUpdateEnrichmentTargets(tc.changes)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMemberUpdateEnrichmentTargets_NickChange(t *testing.T) {
	got := memberUpdateEnrichmentTargets([]discord.AuditLogChange{
		{Key: discord.AuditLogChangeKeyNick, NewValue: []byte(`"new nick"`)},
	})
	assert.Equal(t, []audit.EventType{audit.EventMemberNickChange}, got)
}

// Role add + remove in one native entry must produce exactly one
// EventMemberRoleChange, not two. The de-dup matters because a stray
// second TryEnrich(MatchFirst) would buffer an enrichment that could
// latch onto the next unrelated gateway event within pendingTTL.
func TestMemberUpdateEnrichmentTargets_RoleAddAndRemoveDedup(t *testing.T) {
	got := memberUpdateEnrichmentTargets([]discord.AuditLogChange{
		{Key: discord.AuditLogChangeKeyRoleAdd, NewValue: []byte(`[{"id":"1","name":"foo"}]`)},
		{Key: discord.AuditLogChangeKeyRoleRemove, NewValue: []byte(`[{"id":"2","name":"bar"}]`)},
	})
	assert.Equal(t, []audit.EventType{audit.EventMemberRoleChange}, got)
}

// A native entry with nick AND timeout in the same payload should produce
// both event types, in the order the changes appeared (nick first).
func TestMemberUpdateEnrichmentTargets_CombinedNickAndTimeout(t *testing.T) {
	got := memberUpdateEnrichmentTargets([]discord.AuditLogChange{
		{Key: discord.AuditLogChangeKeyNick, NewValue: []byte(`"new nick"`)},
		{Key: discord.AuditLogChangeKeyCommunicationDisabledUntil, NewValue: []byte(`"2025-01-01T00:00:00Z"`)},
	})
	assert.Equal(t, []audit.EventType{audit.EventMemberNickChange, audit.EventMemberTimeoutAdd}, got)
}

// Empty / unrecognised change keys fall back to the legacy EventMemberUpdate.
// Otherwise unmapped Discord changes would silently drop the actor
// attribution.
func TestMemberUpdateEnrichmentTargets_FallbackOnUnknownKeys(t *testing.T) {
	got := memberUpdateEnrichmentTargets([]discord.AuditLogChange{
		{Key: discord.AuditLogChangeKey("unmapped_field"), NewValue: []byte(`"x"`)},
	})
	assert.Equal(t, []audit.EventType{audit.EventMemberUpdate}, got)

	got = memberUpdateEnrichmentTargets(nil)
	assert.Equal(t, []audit.EventType{audit.EventMemberUpdate}, got)
}

func TestAuditEntryCount(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		in   *string
		want int
	}{
		{"nil", nil, 0},
		{"empty", str(""), 0},
		{"unparseable", str("not a number"), 0},
		{"negative", str("-5"), 0},
		{"zero", str("0"), 0},
		{"positive", str("42"), 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, auditEntryCount(tc.in))
		})
	}
}

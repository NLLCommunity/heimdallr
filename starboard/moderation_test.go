package starboard

import (
	"testing"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
)

func TestUnrelatedExclusionDoesNotRepostDownvotedMessage(t *testing.T) {
	for _, action := range []string{"blacklist-message", "unblacklist-message", "blacklist-channel", "unblacklist-channel"} {
		for _, global := range []bool{false, true} {
			t.Run(action+"/"+map[bool]string{false: "board", true: "guild"}[global], func(t *testing.T) {
				s, f, b := setup(t)
				f.votes[voteKey(100, b.Emoji)] = Votes{Positive: []snowflake.ID{1, 2, 3, 4}}
				reconcile(t, s)
				f.votes[voteKey(entry(t, s, b).CopyMessageID, b.Emoji)] = Votes{Negative: []snowflake.ID{5}}
				reconcile(t, s)
				require.Zero(t, entry(t, s, b).CopyMessageID)
				f.messages[200] = &discord.Message{ID: 200, ChannelID: 31, Author: discord.User{ID: 50}}
				scope := b.ID
				if global {
					scope = 0
				}
				require.NoError(t, s.Moderate(10, scope, action, 31, 200))
				require.NoError(t, s.reconcile(10, 30, 100, false, false))
				require.Len(t, f.published, 1, "unrelated exclusion must not resurrect the downvoted copy")
			})
		}
	}
}

func TestExclusionResetsOnlyAffectedEntries(t *testing.T) {
	for _, kind := range []string{"message", "channel"} {
		for _, global := range []bool{false, true} {
			t.Run(kind+"/"+map[bool]string{false: "board", true: "guild"}[global], func(t *testing.T) {
				s, _, b := setup(t)
				entries := []model.StarboardEntry{
					{GuildID: 10, BoardID: b.ID, ChannelID: 30, MessageID: 100, CopyChannelID: 20, CopyMessageID: 1000},
					{GuildID: 10, BoardID: b.ID, ChannelID: 31, ParentChannelID: 30, MessageID: 101},
					{GuildID: 10, BoardID: b.ID, ChannelID: 32, MessageID: 102},
					{GuildID: 10, BoardID: b.ID + 1, ChannelID: 30, MessageID: 100},
					{GuildID: 11, BoardID: b.ID + 2, ChannelID: 30, MessageID: 100},
				}
				for i := range entries {
					entries[i].WaitFingerprint = "waiting"
					entries[i].Suppressed = true
				}
				require.NoError(t, s.db.Create(&entries).Error)
				scope := b.ID
				if global {
					scope = 0
				}
				channel, message := snowflake.ID(30), snowflake.ID(0)
				if kind == "message" {
					channel, message = 20, 1000
				} // Resolve a copy to its source.
				require.NoError(t, s.Moderate(10, scope, "unblacklist-"+kind, channel, message))
				for i, original := range entries {
					var got model.StarboardEntry
					require.NoError(t, s.db.First(&got, original.ID).Error)
					affected := i == 0 || (i == 1 && kind == "channel") || (i == 3 && global)
					if affected {
						require.Empty(t, got.WaitFingerprint)
					} else {
						require.Equal(t, "waiting", got.WaitFingerprint)
					}
					require.True(t, got.Suppressed, "exclusions must preserve moderator removal")
				}
			})
		}
	}
}

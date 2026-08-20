package prune

import (
	"fmt"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterInstallsPruneRoutes(t *testing.T) {
	router := handler.New()
	Interactions(router)
	require.True(t, router.Match("/button/prune-members/confirm/id", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	require.True(t, router.Match("/button/prune-members/cancel/id", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
}

func makeMembers(n int) []discord.Member {
	members := make([]discord.Member, 0, n)
	for i := range n {
		members = append(members, discord.Member{
			User: discord.User{
				ID:       snowflake.ID(100000000000000000 + i),
				Username: fmt.Sprintf("member-with-a-long-username-%04d", i),
			},
		})
	}
	return members
}

func TestBuildPruneConfirmMessages(t *testing.T) {
	pruneID := uuid.New()

	t.Run("few members fit in a single list message plus prompt", func(t *testing.T) {
		messages, err := buildPruneConfirmMessages(pruneID, makeMembers(3))
		require.NoError(t, err)
		require.Len(t, messages, 2)
	})

	t.Run("no message content exceeds the Discord limit", func(t *testing.T) {
		messages, err := buildPruneConfirmMessages(pruneID, makeMembers(500))
		require.NoError(t, err)
		require.Greater(t, len(messages), 1)
		for i, msg := range messages {
			assert.LessOrEqual(t, len(msg.Content), 2000, "message %d exceeds 2000 chars", i)
		}
	})

	t.Run("every member is listed across the messages", func(t *testing.T) {
		members := makeMembers(500)
		messages, err := buildPruneConfirmMessages(pruneID, members)
		require.NoError(t, err)

		var all strings.Builder
		for _, msg := range messages {
			all.WriteString(msg.Content)
		}
		joined := all.String()

		for _, member := range members {
			assert.Contains(t, joined, member.User.ID.String())
		}
	})

	t.Run("only the last message has the confirm and cancel buttons", func(t *testing.T) {
		messages, err := buildPruneConfirmMessages(pruneID, makeMembers(500))
		require.NoError(t, err)

		for i, msg := range messages[:len(messages)-1] {
			assert.Empty(t, msg.Components, "message %d should not have components", i)
		}

		last := messages[len(messages)-1]
		require.NotEmpty(t, last.Components)
		assert.Contains(t, componentCustomIDs(t, last), "/button/prune-members/confirm/"+pruneID.String())
		assert.Contains(t, componentCustomIDs(t, last), "/button/prune-members/cancel/"+pruneID.String())
	})

	t.Run("button message stays short enough to append to later", func(t *testing.T) {
		messages, err := buildPruneConfirmMessages(pruneID, makeMembers(500))
		require.NoError(t, err)
		last := messages[len(messages)-1]
		// PruneCancelHandler appends to this message on cancel; it must have
		// plenty of headroom below the 2000 char limit.
		assert.Less(t, len(last.Content), 500)
	})

	t.Run("all messages are ephemeral", func(t *testing.T) {
		messages, err := buildPruneConfirmMessages(pruneID, makeMembers(500))
		require.NoError(t, err)
		for i, msg := range messages {
			assert.NotZero(t, msg.Flags&discord.MessageFlagEphemeral, "message %d is not ephemeral", i)
		}
	})
}

func componentCustomIDs(t *testing.T, msg discord.MessageCreate) []string {
	t.Helper()
	var ids []string
	for _, layout := range msg.Components {
		row, ok := layout.(discord.ActionRowComponent)
		if !ok {
			continue
		}
		for _, component := range row.Components {
			if button, ok := component.(discord.ButtonComponent); ok {
				ids = append(ids, button.CustomID)
			}
		}
	}
	return ids
}

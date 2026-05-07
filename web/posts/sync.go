package posts

import (
	"github.com/disgoorg/snowflake/v2"
)

// DiscordClient is the narrow interface the sync engine needs. The
// production wrapper lives in web/posts/discord_client.go (Task 16) and
// adapts the disgo REST client to this interface.
type DiscordClient interface {
	SendV2(channelID snowflake.ID, chunk []any) (snowflake.ID, error)
	EditV2(channelID, messageID snowflake.ID, chunk []any) error
	Delete(channelID, messageID snowflake.ID) error
}

// ExistingMessage is one of the post's currently-published Discord messages.
type ExistingMessage struct {
	ChannelID snowflake.ID
	MessageID snowflake.ID
}

// CreatedMessage is a freshly-sent message; the caller persists these as
// PostMessage rows.
type CreatedMessage struct {
	ChannelID snowflake.ID
	MessageID snowflake.ID
}

// SyncPlan is the input to Sync(): the new chunks to publish and the target
// channel for new sends.
type SyncPlan struct {
	NewChunks [][]any
	ChannelID snowflake.ID
}

// SyncResult tells the caller what changed on Discord:
//   - Created: messages newly sent (caller must persist as PostMessage rows)
//   - KeptCount: how many of `existing` were edited in place
//   - DeletedCount: how many trailing existing rows were deleted
//   - RecreatedAll: true when the engine deleted everything and re-sent
type SyncResult struct {
	Created      []CreatedMessage
	KeptCount    int
	DeletedCount int
	RecreatedAll bool
}

// Sync executes the publish-or-update operation against Discord.
func Sync(c DiscordClient, plan SyncPlan, existing []ExistingMessage) (SyncResult, error) {
	if isRetargeted(plan.ChannelID, existing) {
		for _, e := range existing {
			_ = c.Delete(e.ChannelID, e.MessageID)
		}
		result, err := firstPublish(c, plan)
		result.RecreatedAll = true
		return result, err
	}

	if len(existing) == 0 {
		return firstPublish(c, plan)
	}

	N := len(plan.NewChunks)
	M := len(existing)

	switch {
	case N == M:
		return editAllInPlace(c, plan, existing)
	case N < M:
		return editAndTrim(c, plan, existing)
	default:
		return recreateAll(c, plan, existing)
	}
}

func isRetargeted(target snowflake.ID, existing []ExistingMessage) bool {
	for _, e := range existing {
		if e.ChannelID != target {
			return true
		}
	}
	return false
}

func recreateAll(c DiscordClient, plan SyncPlan, existing []ExistingMessage) (SyncResult, error) {
	for _, e := range existing {
		_ = c.Delete(e.ChannelID, e.MessageID)
	}
	out := SyncResult{RecreatedAll: true}
	for _, chunk := range plan.NewChunks {
		id, err := c.SendV2(plan.ChannelID, chunk)
		if err != nil {
			return out, err
		}
		out.Created = append(out.Created, CreatedMessage{
			ChannelID: plan.ChannelID,
			MessageID: id,
		})
	}
	return out, nil
}

func editAndTrim(c DiscordClient, plan SyncPlan, existing []ExistingMessage) (SyncResult, error) {
	for i, chunk := range plan.NewChunks {
		if err := c.EditV2(existing[i].ChannelID, existing[i].MessageID, chunk); err != nil {
			return SyncResult{}, err
		}
	}
	deleted := 0
	for i := len(plan.NewChunks); i < len(existing); i++ {
		_ = c.Delete(existing[i].ChannelID, existing[i].MessageID)
		deleted++
	}
	return SyncResult{
		KeptCount:    len(plan.NewChunks),
		DeletedCount: deleted,
	}, nil
}

func editAllInPlace(c DiscordClient, plan SyncPlan, existing []ExistingMessage) (SyncResult, error) {
	for i, chunk := range plan.NewChunks {
		if err := c.EditV2(existing[i].ChannelID, existing[i].MessageID, chunk); err != nil {
			return SyncResult{}, err
		}
	}
	return SyncResult{KeptCount: len(plan.NewChunks)}, nil
}

func firstPublish(c DiscordClient, plan SyncPlan) (SyncResult, error) {
	out := SyncResult{}
	for _, chunk := range plan.NewChunks {
		id, err := c.SendV2(plan.ChannelID, chunk)
		if err != nil {
			return out, err
		}
		out.Created = append(out.Created, CreatedMessage{
			ChannelID: plan.ChannelID,
			MessageID: id,
		})
	}
	return out, nil
}

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
//   - DeletedCount: how many of `existing` were deleted — trailing rows in
//     the edit-and-trim case, all rows in the recreate/retarget cases
//   - RecreatedAll: true when the engine deleted everything and re-sent
//   - DeleteFailures: best-effort deletes that errored on Discord — typically
//     because the message was already gone or permissions changed. Surfaced
//     to the caller so it can log specific IDs for manual cleanup; the sync
//     engine itself ignores them on the assumption that "we wanted it gone
//     and now it's gone (or never was there)" is fine.
type SyncResult struct {
	Created        []CreatedMessage
	KeptCount      int
	DeletedCount   int
	RecreatedAll   bool
	DeleteFailures []DeleteFailure
}

// DeleteFailure is a single Discord delete error captured by Sync. ChannelID
// + MessageID identify the (now-orphaned-or-already-gone) message; Err is
// the underlying REST error.
type DeleteFailure struct {
	ChannelID snowflake.ID
	MessageID snowflake.ID
	Err       error
}

// Sync executes the publish-or-update operation against Discord.
func Sync(c DiscordClient, plan SyncPlan, existing []ExistingMessage) (SyncResult, error) {
	if isRetargeted(plan.ChannelID, existing) {
		var failures []DeleteFailure
		for _, e := range existing {
			if err := c.Delete(e.ChannelID, e.MessageID); err != nil {
				failures = append(failures, DeleteFailure{ChannelID: e.ChannelID, MessageID: e.MessageID, Err: err})
			}
		}
		result, err := firstPublish(c, plan)
		result.RecreatedAll = true
		result.DeletedCount = len(existing)
		result.DeleteFailures = failures
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
	out := SyncResult{RecreatedAll: true, DeletedCount: len(existing)}
	for _, e := range existing {
		if err := c.Delete(e.ChannelID, e.MessageID); err != nil {
			out.DeleteFailures = append(out.DeleteFailures, DeleteFailure{ChannelID: e.ChannelID, MessageID: e.MessageID, Err: err})
		}
	}
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
	out := SyncResult{KeptCount: len(plan.NewChunks)}
	for i := len(plan.NewChunks); i < len(existing); i++ {
		if err := c.Delete(existing[i].ChannelID, existing[i].MessageID); err != nil {
			out.DeleteFailures = append(out.DeleteFailures, DeleteFailure{ChannelID: existing[i].ChannelID, MessageID: existing[i].MessageID, Err: err})
		}
		out.DeletedCount++
	}
	return out, nil
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

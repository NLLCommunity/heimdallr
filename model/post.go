package model

import (
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

// Post is the editable, versioned source-of-truth for a piece of long-form
// bot content. ComponentsJSON holds a top-level array of V2 components, the
// same shape the message-builder editor produces.
type Post struct {
	ID             uint         `gorm:"primaryKey"`
	GuildID        snowflake.ID `gorm:"index;not null"`
	Name           string       `gorm:"not null"`
	ChannelID      snowflake.ID
	ComponentsJSON string `gorm:"type:text;not null"`
	Version        uint   `gorm:"not null;default:1"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	UpdatedBy      snowflake.ID
}

// PostMessage tracks the Discord messages currently owned by a post, in
// publication order. Position is 0-indexed and dense.
type PostMessage struct {
	ID        uint         `gorm:"primaryKey"`
	PostID    uint         `gorm:"index;not null"`
	Position  int          `gorm:"not null"`
	ChannelID snowflake.ID `gorm:"not null"`
	MessageID snowflake.ID `gorm:"not null"`
	CreatedAt time.Time
}

// ErrPostStaleVersion is returned when an optimistic version check fails.
// Callers should surface this as a 409 Conflict; the post in the DB has been
// updated by another writer since this caller loaded it.
var ErrPostStaleVersion = errors.New("post has been updated by another writer")

// CreatePost inserts a new post and returns it with ID + Version populated.
// Pass 0 for channelID to leave the post without a target channel.
func CreatePost(guildID snowflake.ID, name, componentsJSON string, channelID, updatedBy snowflake.ID) (*Post, error) {
	p := &Post{
		GuildID:        guildID,
		Name:           name,
		ChannelID:      channelID,
		ComponentsJSON: componentsJSON,
		Version:        1,
		UpdatedBy:      updatedBy,
	}
	if err := DB.Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// GetPost loads a post by ID, scoped to the guild for safety.
func GetPost(guildID snowflake.ID, id uint) (*Post, error) {
	var p Post
	if err := DB.Where("guild_id = ? AND id = ?", guildID, id).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPosts returns all posts for a guild, ordered by most-recently-updated.
func ListPosts(guildID snowflake.ID) ([]Post, error) {
	var posts []Post
	if err := DB.Where("guild_id = ?", guildID).Order("updated_at DESC").Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

// UpdatePostFields runs an optimistic-locked UPDATE: succeeds only if
// expectedVersion matches the row's current Version, and bumps Version by 1.
// Returns gorm.ErrRecordNotFound when no post matches (guildID, id), and
// ErrPostStaleVersion when the post exists but has been bumped since.
func UpdatePostFields(guildID snowflake.ID, id, expectedVersion uint, name, componentsJSON string, channelID snowflake.ID, updatedBy snowflake.ID) (*Post, error) {
	res := DB.Model(&Post{}).
		Where("guild_id = ? AND id = ? AND version = ?", guildID, id, expectedVersion).
		Updates(map[string]any{
			"name":            name,
			"components_json": componentsJSON,
			"channel_id":      channelID,
			"updated_by":      updatedBy,
			"version":         gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// 0 rows can mean either "no such post in this guild" or "version
		// drifted". A second SELECT disambiguates so callers can return a
		// 404 vs 409 instead of conflating them.
		var existing Post
		if err := DB.Where("guild_id = ? AND id = ?", guildID, id).First(&existing).Error; err != nil {
			return nil, err
		}
		return nil, ErrPostStaleVersion
	}
	return GetPost(guildID, id)
}

// DeletePost removes the post and its PostMessage rows. Returns
// gorm.ErrRecordNotFound if no post matches the (guildID, id) tuple, so
// callers can distinguish "deleted" from "didn't exist or wrong guild".
// Discord-side cleanup is the caller's responsibility (see web/posts/sync.go).
func DeletePost(guildID snowflake.ID, id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// Verify ownership first; fail fast if the post doesn't belong to
		// this guild. This avoids deleting another guild's PostMessage
		// rows in the (vanishingly rare) case of a guessed post ID that
		// belongs to a different guild — the rollback would protect us
		// anyway, but ordering the checks correctly is cheaper and clearer.
		res := tx.Where("guild_id = ? AND id = ?", guildID, id).Delete(&Post{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Where("post_id = ?", id).Delete(&PostMessage{}).Error
	})
}

// ListPostMessages returns the post's Discord-message rows in position order.
// The guild_id parameter is enforced by joining through the posts table so a
// caller can't enumerate messages for a post in another guild via raw post IDs.
func ListPostMessages(guildID snowflake.ID, postID uint) ([]PostMessage, error) {
	var msgs []PostMessage
	err := DB.
		Joins("JOIN posts ON posts.id = post_messages.post_id").
		Where("posts.guild_id = ? AND post_messages.post_id = ?", guildID, postID).
		Order("post_messages.position ASC").
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

// ReplacePostMessages atomically deletes existing PostMessage rows for the
// post and inserts the new set. Used by the sync algorithm whenever the
// message set changes wholesale (first publish, recreate, or re-target).
func ReplacePostMessages(postID uint, msgs []PostMessage) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("post_id = ?", postID).Delete(&PostMessage{}).Error; err != nil {
			return err
		}
		for i := range msgs {
			msgs[i].PostID = postID
			msgs[i].Position = i
		}
		if len(msgs) == 0 {
			return nil
		}
		return tx.Create(&msgs).Error
	})
}

// PostListEntry pairs a Post with its current published-message count.
// Used by the posts list view to render Draft vs Published correctly.
type PostListEntry struct {
	Post         Post
	MessageCount int
}

// ListPostsWithCounts returns all posts for a guild with each post's current
// PostMessage count, ordered by most-recently-updated. A MessageCount of zero
// means the post is a draft (not currently published to Discord).
func ListPostsWithCounts(guildID snowflake.ID) ([]PostListEntry, error) {
	type row struct {
		Post
		MessageCount int
	}
	var rows []row
	err := DB.
		Table("posts").
		Select("posts.*, COUNT(post_messages.id) AS message_count").
		Joins("LEFT JOIN post_messages ON post_messages.post_id = posts.id").
		Where("posts.guild_id = ?", guildID).
		Group("posts.id").
		Order("posts.updated_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]PostListEntry, len(rows))
	for i, r := range rows {
		out[i] = PostListEntry{Post: r.Post, MessageCount: r.MessageCount}
	}
	return out, nil
}

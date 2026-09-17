package model

import (
	"github.com/disgoorg/snowflake/v2"
	"time"
)

// Starboard belongs to one guild. Deleting is a durable cleanup request.
type Starboard struct {
	ID        uint64       `gorm:"primaryKey"`
	GuildID   snowflake.ID `gorm:"uniqueIndex:starboard_name;uniqueIndex:starboard_channel;index;not null"`
	Name      string       `gorm:"uniqueIndex:starboard_name;not null"`
	ChannelID snowflake.ID `gorm:"uniqueIndex:starboard_channel;not null"`
	Emoji     string
	Threshold int
	Enabled   bool
	Deleting  bool
	Revision  uint64
}

// StarboardEntry survives unpublishing so moderator decisions and the source
// voter fingerprint cannot be lost when Discord deletes a copy's reactions.
type StarboardEntry struct {
	ParentChannelID snowflake.ID

	ID              uint64       `gorm:"primaryKey"`
	GuildID         snowflake.ID `gorm:"index;uniqueIndex:starboard_entry"`
	BoardID         uint64       `gorm:"index;uniqueIndex:starboard_entry"`
	ChannelID       snowflake.ID
	MessageID       snowflake.ID `gorm:"index;uniqueIndex:starboard_entry"`
	CopyChannelID   snowflake.ID
	CopyMessageID   snowflake.ID `gorm:"index"`
	Suppressed      bool
	SourceDeleted   bool
	WaitFingerprint string
	Revision        uint64
	Positive        int
	Negative        int
	SendNonce       string
	SendStartedAt   *time.Time
}

// BoardID zero applies an exclusion to every board in the guild.
type StarboardExclusion struct {
	ID       uint64       `gorm:"primaryKey"`
	GuildID  snowflake.ID `gorm:"uniqueIndex:starboard_exclusion;index"`
	BoardID  uint64       `gorm:"uniqueIndex:starboard_exclusion"`
	Kind     string       `gorm:"uniqueIndex:starboard_exclusion"`
	TargetID snowflake.ID `gorm:"uniqueIndex:starboard_exclusion"`
}

// StarboardCleanup is written before deleting a Discord copy. It separates
// service deletion from moderator suppression, including after a restart.
type StarboardCleanup struct {
	ID        uint64       `gorm:"primaryKey"`
	GuildID   snowflake.ID `gorm:"index"`
	ChannelID snowflake.ID
	MessageID snowflake.ID `gorm:"uniqueIndex"`
}

// StarboardJob coalesces gateway bursts while persisting failed work. Token
// prevents a worker from consuming a newer notification arriving during REST.
type StarboardJob struct {
	ID          uint64       `gorm:"primaryKey"`
	GuildID     snowflake.ID `gorm:"uniqueIndex:starboard_job"`
	ChannelID   snowflake.ID `gorm:"uniqueIndex:starboard_job"`
	MessageID   snowflake.ID `gorm:"uniqueIndex:starboard_job"`
	Deleted     bool
	Discover    bool
	Token       string
	Attempts    int
	NextAttempt time.Time `gorm:"index"`
	LastError   string
}

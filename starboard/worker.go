package starboard

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Notify persists a reaction/deletion notification. No Discord request is made
// on the gateway callback. A full channel or a process restart cannot lose it.
func (s *Service) Notify(guild, channel, message snowflake.ID, deleted bool) {
	s.notify(guild, channel, message, deleted, true)
}

// NotifyUpdate edits only already tracked entries; unrelated message edits
// must not create a history scan or grow the starboard discovery queue.
func (s *Service) NotifyUpdate(guild, channel, message snowflake.ID) {
	// Only originals can trigger source refreshes. Editing a bot copy must
	// not recursively enqueue another copy edit forever.
	var n int64
	if err := s.db.Model(&model.StarboardEntry{}).Where("guild_id = ? AND channel_id = ? AND message_id = ?", guild, channel, message).Count(&n).Error; err != nil {
		slog.Error("starboard update lookup", "error", err)
		return
	}
	if n != 0 {
		s.notify(guild, channel, message, false, false)
	}
}
func (s *Service) notify(guild, channel, message snowflake.ID, deleted, discover bool) {
	if guild == 0 || channel == 0 || message == 0 {
		return
	}
	var n int64
	if discover && !deleted {
		if err := s.db.Model(&model.GuildSettings{}).Where("guild_id = ? AND starboard_enabled = ?", guild, true).Count(&n).Error; err != nil {
			slog.Error("starboard settings lookup", "error", err)
			return
		}
	}
	if n == 0 {
		if err := s.db.Model(&model.StarboardEntry{}).Where("guild_id = ? AND (message_id = ? OR copy_message_id = ?)", guild, message, message).Count(&n).Error; err != nil {
			slog.Error("starboard entry lookup", "error", err)
			return
		}
		if n == 0 && deleted {
			if err := s.db.Model(&model.StarboardJob{}).Where("guild_id = ? AND channel_id = ? AND message_id = ?", guild, channel, message).Count(&n).Error; err != nil {
				slog.Error("starboard pending deletion lookup", "error", err)
				return
			}
		}
		if n == 0 {
			return
		}
	}
	if err := s.enqueue(s.db, guild, channel, message, deleted, discover); err != nil {
		slog.Error("starboard event persistence", "guild", guild, "message", message, "error", err)
	}
}
func (s *Service) enqueue(db *gorm.DB, guild, channel, message snowflake.ID, deleted, discover bool) error {
	job := model.StarboardJob{GuildID: guild, ChannelID: channel, MessageID: message, Deleted: deleted, Discover: discover, Token: uuid.NewString(), NextAttempt: time.Now().Add(750 * time.Millisecond)}
	err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "guild_id"}, {Name: "channel_id"}, {Name: "message_id"}}, DoUpdates: clause.Assignments(map[string]any{
		"deleted": gorm.Expr("starboard_jobs.deleted OR ?", deleted), "discover": gorm.Expr("starboard_jobs.discover OR ?", discover), "token": job.Token,
		// Do not push the debounce deadline back indefinitely on busy messages,
		// and do not bypass retry backoff when more events arrive.
	})}).Create(&job).Error
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return err
}

// Explicit moderation/settings changes should not wait behind an old REST
// failure's backoff. Ordinary reaction notifications retain that backoff.
func (s *Service) enqueueImmediate(db *gorm.DB, guild, channel, message snowflake.ID) error {
	if err := s.enqueue(db, guild, channel, message, false, false); err != nil {
		return err
	}
	return db.Model(&model.StarboardJob{}).Where("guild_id = ? AND channel_id = ? AND message_id = ?", guild, channel, message).Updates(map[string]any{"next_attempt": time.Now(), "attempts": 0}).Error
}

func (s *Service) queueEntries(db *gorm.DB, guild snowflake.ID, board uint64) error {
	q := db.Where("guild_id = ?", guild)
	if board != 0 {
		q = q.Where("board_id = ?", board)
	}
	var entries []model.StarboardEntry
	if err := q.Find(&entries).Error; err != nil {
		return err
	}
	for _, e := range entries {
		if err := s.enqueueImmediate(db, e.GuildID, e.ChannelID, e.MessageID); err != nil {
			return err
		}
	}
	return nil
}

// Run uses one bounded batch per tick, leaving room for gateway work and REST
// rate limiting. Persistent retries back off to at most fifteen minutes.
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	repair := time.NewTicker(5 * time.Minute)
	defer repair.Stop()
	s.repair(0)
	var cursor uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-tick.C:
		case <-repair.C:
			cursor = s.repair(cursor)
		}
		var jobs []model.StarboardJob
		if err := s.db.Where("next_attempt <= ?", time.Now()).Order("id").Limit(25).Find(&jobs).Error; err != nil {
			slog.Error("starboard jobs", "error", err)
			continue
		}
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			err := s.reconcile(job.GuildID, job.ChannelID, job.MessageID, job.Deleted, job.Discover)
			q := s.db.Where("id = ? AND token = ?", job.ID, job.Token)
			if err == nil {
				if e := q.Delete(&model.StarboardJob{}).Error; e != nil {
					slog.Error("starboard finish job", "error", e)
				}
			} else {
				delay := time.Second * time.Duration(1<<min(job.Attempts+1, 10))
				if delay > 15*time.Minute {
					delay = 15 * time.Minute
				}
				saveErr := q.Model(&model.StarboardJob{}).Updates(map[string]any{"attempts": job.Attempts + 1, "next_attempt": time.Now().Add(delay), "last_error": err.Error()}).Error
				slog.Warn("starboard reconcile retry", "guild", job.GuildID, "message", job.MessageID, "error", errors.Join(err, saveErr))
			}
		}
		s.finishDeletedBoards()
	}
}

func (s *Service) repair(cursor uint64) uint64 {
	// Round-robin rather than repeatedly checking the first page forever.
	var entries []model.StarboardEntry
	if err := s.db.Where("id > ? AND source_deleted = ?", cursor, false).Order("id").Limit(100).Find(&entries).Error; err != nil {
		slog.Error("starboard repair", "error", err)
		return cursor
	}
	for _, e := range entries {
		if err := s.enqueue(s.db, e.GuildID, e.ChannelID, e.MessageID, false, false); err != nil {
			slog.Error("starboard repair enqueue", "error", err)
		}
	}
	if len(entries) < 100 {
		return 0
	}
	return entries[len(entries)-1].ID
}
func (s *Service) finishDeletedBoards() {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var boards []model.Starboard
		if err := tx.Where("deleting = ?", true).Find(&boards).Error; err != nil {
			return err
		}
		for _, b := range boards {
			var n int64
			if err := tx.Model(&model.StarboardEntry{}).Where("guild_id = ? AND board_id = ? AND (copy_message_id <> 0 OR send_nonce <> '')", b.GuildID, b.ID).Count(&n).Error; err != nil {
				return err
			}
			if n != 0 {
				continue
			}
			if err := tx.Where("guild_id = ? AND board_id = ?", b.GuildID, b.ID).Delete(&model.StarboardEntry{}).Error; err != nil {
				return err
			}
			if err := tx.Where("guild_id = ? AND board_id = ?", b.GuildID, b.ID).Delete(&model.StarboardExclusion{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&b).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("starboard board cleanup", "error", err)
	}
}

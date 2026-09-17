package starboard

import (
	"errors"
	"fmt"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

func (s *Service) Moderate(guild snowflake.ID, board uint64, action string, channel, message snowflake.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if guild == 0 {
		return errors.New("guild required")
	}
	var b model.Starboard
	if board != 0 {
		if err := s.db.Where("guild_id = ? AND id = ?", guild, board).First(&b).Error; err != nil {
			return err
		}
	}
	switch action {
	case "remove", "restore":
		if board == 0 || channel == 0 || message == 0 {
			return errors.New("select a board and a message")
		}
		var e model.StarboardEntry
		err := s.db.Where("guild_id = ? AND board_id = ?", guild, board).Where("(channel_id = ? AND message_id = ?) OR (copy_channel_id = ? AND copy_message_id = ?)", channel, message, channel, message).First(&e).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			source, err := s.transport.Source(guild, channel, message)
			if err != nil {
				return err
			}
			e = model.StarboardEntry{GuildID: guild, BoardID: board, ChannelID: source.ChannelID, MessageID: source.ID, Revision: b.Revision}
		} else if err != nil {
			return err
		}
		e.Suppressed = action == "remove"
		if action == "restore" {
			e.WaitFingerprint = ""
		}
		return s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&e).Error; err != nil {
				return err
			}
			return s.enqueueImmediate(tx, guild, e.ChannelID, e.MessageID)
		})
	case "blacklist-message", "unblacklist-message", "blacklist-channel", "unblacklist-channel":
		kind := "channel"
		target := channel
		if strings.HasSuffix(action, "message") {
			kind = "message"
			target = message
			if channel == 0 || message == 0 {
				return errors.New("a message link is required")
			}
			var e model.StarboardEntry
			q := s.db.Where("guild_id = ?", guild).Where("(copy_channel_id = ? AND copy_message_id = ?) OR (channel_id = ? AND message_id = ?)", channel, message, channel, message)
			if board != 0 {
				q = q.Where("board_id = ?", board)
			}
			err := q.First(&e).Error
			if err == nil {
				target = e.MessageID
				channel = e.ChannelID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else {
				// Validate the source belongs to the guild rather than accepting a
				// forged link with this guild's prefix and another guild's channel.
				if _, err = s.transport.Source(guild, channel, message); err != nil {
					return err
				}
			}
		}
		if target == 0 {
			return errors.New("message or channel required")
		}
		return s.db.Transaction(func(tx *gorm.DB) error {
			x := model.StarboardExclusion{GuildID: guild, BoardID: board, Kind: kind, TargetID: target}
			if strings.HasPrefix(action, "unblacklist") {
				if err := tx.Where("guild_id = ? AND board_id = ? AND kind = ? AND target_id = ?", guild, board, kind, target).Delete(&model.StarboardExclusion{}).Error; err != nil {
					return err
				}
			} else if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&x).Error; err != nil {
				return err
			}
			// Clear threshold waiting, but preserve moderator suppression.
			entries := tx.Model(&model.StarboardEntry{}).Where("guild_id = ?", guild)
			if board != 0 {
				entries = entries.Where("board_id = ?", board)
			}
			if err := entries.Update("wait_fingerprint", "").Error; err != nil {
				return err
			}
			return s.queueEntries(tx, guild, board)
		})
	default:
		return fmt.Errorf("unknown moderation action %q", action)
	}
}

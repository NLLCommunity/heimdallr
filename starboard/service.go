package starboard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Default is initialized once, before gateway and HTTP handlers start.
var Default *Service

type Service struct {
	db        *gorm.DB
	transport Transport
	// Management and reconciliation share the lock: no publish can race a
	// moderator removal or a settings change. Gateway ingestion is independent.
	mu   sync.Mutex
	wake chan struct{}
}

func New(db *gorm.DB, transport Transport) *Service {
	return &Service{db: db, transport: transport, wake: make(chan struct{}, 1)}
}

func (s *Service) SaveBoard(b *model.Starboard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.GuildID == 0 || b.ChannelID == 0 || b.Threshold < 1 || b.Threshold > 100000 {
		return errors.New("a guild, destination and threshold between 1 and 100000 are required")
	}
	b.Name = strings.TrimSpace(b.Name)
	if len([]rune(b.Name)) < 1 || len([]rune(b.Name)) > 80 {
		return errors.New("board name must contain 1–80 characters")
	}
	emoji, err := ParseEmoji(b.Emoji)
	if err != nil {
		return err
	}
	b.Emoji = emoji
	var previous model.Starboard
	if b.ID != 0 {
		if err = s.db.Where("id = ? AND guild_id = ?", b.ID, b.GuildID).First(&previous).Error; err != nil {
			return err
		}
		if previous.Deleting {
			return errors.New("board is being deleted")
		}
	}
	disablingExisting := previous.ID != 0 && !b.Enabled && previous.ChannelID == b.ChannelID && sameEmoji(previous.Emoji, b.Emoji)
	if !disablingExisting {
		if err = s.transport.ValidateBoard(*b); err != nil {
			return err
		}
	}
	b.Deleting = false
	b.Revision = previous.Revision + 1
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(b).Error; err != nil {
			return fmt.Errorf("save board (names and destinations must be unique): %w", err)
		}
		// A generation mismatch removes an old copy before rebuilding in a new
		// destination or with a different reaction. Threshold/toggle changes do
		// not discard its votes.
		if previous.ID != 0 && previous.ChannelID == b.ChannelID && sameEmoji(previous.Emoji, b.Emoji) {
			if err := tx.Model(&model.StarboardEntry{}).Where("guild_id = ? AND board_id = ? AND revision = ?", b.GuildID, b.ID, previous.Revision).Updates(map[string]any{"revision": b.Revision, "wait_fingerprint": ""}).Error; err != nil {
				return err
			}
		}
		return s.queueEntries(tx, b.GuildID, b.ID)
	})
}
func sameEmoji(a, b string) bool {
	ai := strings.LastIndex(a, ":")
	bi := strings.LastIndex(b, ":")
	if ai >= 0 && bi >= 0 {
		return a[ai+1:] == b[bi+1:]
	}
	return a == b
}

func (s *Service) SetEnabled(guild snowflake.ID, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if guild == 0 {
		return errors.New("guild required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		settings := model.GuildSettings{GuildID: guild}
		if err := tx.FirstOrCreate(&settings, "guild_id = ?", guild).Error; err != nil {
			return err
		}
		if err := tx.Model(&settings).Update("starboard_enabled", enabled).Error; err != nil {
			return err
		}
		return s.queueEntries(tx, guild, 0)
	})
}
func (s *Service) DeleteBoard(guild snowflake.ID, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var b model.Starboard
		if err := tx.Where("guild_id = ? AND id = ?", guild, id).First(&b).Error; err != nil {
			return err
		}
		if err := tx.Model(&b).Updates(map[string]any{"deleting": true, "enabled": false}).Error; err != nil {
			return err
		}
		return s.queueEntries(tx, guild, id)
	})
}

func unionVotes(votes ...Votes) Votes {
	result := Votes{}
	pos, neg := map[snowflake.ID]bool{}, map[snowflake.ID]bool{}
	for _, v := range votes {
		for _, id := range v.Positive {
			if id != 0 {
				pos[id] = true
			}
		}
		for _, id := range v.Negative {
			if id != 0 {
				neg[id] = true
			}
		}
	}
	for id := range pos {
		result.Positive = append(result.Positive, id)
	}
	for id := range neg {
		result.Negative = append(result.Negative, id)
	}
	slices.Sort(result.Positive)
	slices.Sort(result.Negative)
	return result
}
func fingerprint(v Votes) string {
	v = unionVotes(v)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v/%v", v.Positive, v.Negative)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) reconcile(guild, channel, message snowflake.ID, deleted, discover bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Copy events belong only to their board, whereas originals fan out.
	var copyEntry model.StarboardEntry
	lookup := s.db.Where("guild_id = ? AND copy_channel_id = ? AND copy_message_id = ?", guild, channel, message).First(&copyEntry).Error
	if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
		return lookup
	}
	var onlyBoard uint64
	if lookup == nil {
		if deleted {
			var count int64
			if err := s.db.Model(&model.StarboardCleanup{}).Where("guild_id = ? AND message_id = ?", guild, message).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				copyEntry.Suppressed = true
			}
			copyEntry.CopyMessageID = 0
			copyEntry.CopyChannelID = 0
			return s.db.Save(&copyEntry).Error
		}
		onlyBoard = copyEntry.BoardID
		channel = copyEntry.ChannelID
		message = copyEntry.MessageID
		discover = false
	}
	if deleted {
		if err := s.db.Model(&model.StarboardEntry{}).Where("guild_id = ? AND channel_id = ? AND message_id = ?", guild, channel, message).Update("source_deleted", true).Error; err != nil {
			return err
		}
	}
	var boards []model.Starboard
	q := s.db.Where("guild_id = ?", guild)
	if onlyBoard != 0 {
		q = q.Where("id = ?", onlyBoard)
	}
	if err := q.Find(&boards).Error; err != nil {
		return err
	}
	var settings model.GuildSettings
	if err := s.db.Where("guild_id = ?", guild).First(&settings).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var source *discord.Message
	var sourceErr error
	sourceLoaded := false
	var combined error
	for _, b := range boards {
		var e model.StarboardEntry
		err := s.db.Where("guild_id = ? AND board_id = ? AND message_id = ?", guild, b.ID, message).First(&e).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !discover || deleted || !settings.StarboardEnabled || !b.Enabled || b.Deleting {
				continue
			}
			// Don't discover any board destination, even another board's.
			var destinations int64
			if err = s.db.Model(&model.Starboard{}).Where("guild_id = ? AND channel_id = ?", guild, channel).Count(&destinations).Error; err != nil {
				return err
			}
			if destinations > 0 {
				continue
			}
			e = model.StarboardEntry{GuildID: guild, BoardID: b.ID, ChannelID: channel, MessageID: message, Revision: b.Revision}
		} else if err != nil {
			return err
		}
		if e.CopyMessageID != 0 {
			var pending int64
			if err := s.db.Model(&model.StarboardCleanup{}).Where("guild_id = ? AND message_id = ?", guild, e.CopyMessageID).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				if err := s.removeCopy(&e); err != nil {
					combined = errors.Join(combined, err)
					continue
				}
			}
		}
		knownExcluded, err := s.excluded(b, message, channel, e.ParentChannelID)
		if err != nil {
			return err
		}
		if e.SourceDeleted || e.Suppressed || b.Deleting || knownExcluded {
			if err = s.reconcileEntry(b, &e, nil, Votes{}, false, false); err != nil {
				combined = errors.Join(combined, err)
			}
			continue
		}
		if !sourceLoaded {
			source, sourceErr = s.transport.Source(guild, channel, message)
			sourceLoaded = true
		}
		if sourceErr != nil {
			if errors.Is(sourceErr, ErrNotFound) {
				e.SourceDeleted = true
				err = s.reconcileEntry(b, &e, nil, Votes{}, false, false)
			} else {
				err = sourceErr
			}
			combined = errors.Join(combined, err)
			continue
		}
		eligible, parent, err := s.transport.Eligible(b, source)
		if err != nil {
			combined = errors.Join(combined, err)
			continue
		}
		var destinations int64
		if err = s.db.Model(&model.Starboard{}).Where("guild_id = ? AND channel_id IN ?", guild, []snowflake.ID{channel, parent}).Count(&destinations).Error; err != nil {
			return err
		}
		if destinations > 0 {
			eligible = false
		}
		e.ParentChannelID = parent
		excluded, err := s.excluded(b, message, channel, parent)
		if err != nil {
			return err
		}
		eligible = eligible && !excluded
		if source.Author.Bot || source.WebhookID != nil {
			eligible = false
		}
		if e.ID == 0 {
			if !eligible {
				continue
			}
			if err = s.db.Create(&e).Error; err != nil {
				return err
			}
		}
		active := settings.StarboardEnabled && b.Enabled
		votes := Votes{}
		if eligible && active {
			votes, err = s.transport.Votes(channel, message, b.Emoji)
			if err != nil {
				combined = errors.Join(combined, err)
				continue
			}
		}
		err = s.reconcileEntry(b, &e, source, votes, eligible, active)
		combined = errors.Join(combined, err)
	}
	return combined
}

func (s *Service) excluded(b model.Starboard, message, channel, parent snowflake.ID) (bool, error) {
	var n int64
	err := s.db.Model(&model.StarboardExclusion{}).Where("guild_id = ? AND board_id IN ?", b.GuildID, []uint64{0, b.ID}).Where("(kind = ? AND target_id = ?) OR (kind = ? AND target_id IN ?)", "message", message, "channel", []snowflake.ID{channel, parent}).Count(&n).Error
	return n > 0, err
}

func (s *Service) reconcileEntry(b model.Starboard, e *model.StarboardEntry, source *discord.Message, original Votes, eligible, active bool) error {
	// Recover first even when a moderator has since removed the entry: a send
	// may already exist on Discord and must be found before it can be deleted.
	if e.SendNonce != "" && e.CopyMessageID == 0 {
		recoveryBoard := b
		recoveryBoard.ChannelID = e.CopyChannelID
		if e.SendStartedAt == nil {
			return ErrAmbiguousSend
		}
		id, err := s.transport.Recover(recoveryBoard, e.SendNonce, *e.SendStartedAt)
		if errors.Is(err, ErrNotSent) {
			e.SendNonce = ""
			e.SendStartedAt = nil
			e.CopyChannelID = 0
			if saveErr := s.db.Save(e).Error; saveErr != nil {
				return saveErr
			}
			// This pass only clears the failed operation; re-evaluate from
			// current source, votes and moderation on the next retry.
			return err
		}
		if err != nil {
			return err
		}
		if id == 0 {
			return ErrAmbiguousSend
		}
		e.CopyMessageID = id
		e.SendNonce = ""
		e.SendStartedAt = nil
		if err = s.db.Save(e).Error; err != nil {
			return err
		}
	}
	remove := e.Suppressed || e.SourceDeleted || b.Deleting || !eligible
	changed := e.Revision != b.Revision
	if remove || changed {
		if err := s.removeCopy(e); err != nil {
			return err
		}
		e.Revision = b.Revision
		e.WaitFingerprint = ""
		if err := s.db.Save(e).Error; err != nil {
			return err
		}
		if remove {
			return nil
		}
	}
	if !active {
		if e.CopyMessageID != 0 && source != nil {
			// Preserve displayed counts while disabled, but never stale source text.
			counts := Votes{Positive: make([]snowflake.ID, e.Positive), Negative: make([]snowflake.ID, e.Negative)}
			return s.updateCopy(b, e, source, counts)
		}
		return nil
	}
	original = unionVotes(original)
	fp := fingerprint(original)
	if e.CopyMessageID == 0 && e.WaitFingerprint == fp {
		return nil
	}
	combined := original
	if e.CopyMessageID != 0 {
		copyVotes, err := s.transport.Votes(e.CopyChannelID, e.CopyMessageID, b.Emoji)
		if errors.Is(err, ErrNotFound) {
			e.Suppressed = true
			e.CopyMessageID = 0
			e.CopyChannelID = 0
			return s.db.Save(e).Error
		}
		if err != nil {
			return err
		}
		combined = unionVotes(original, copyVotes)
	}
	e.Positive = len(combined.Positive)
	e.Negative = len(combined.Negative)
	if e.Positive-e.Negative < b.Threshold {
		if e.CopyMessageID != 0 {
			e.WaitFingerprint = fp
			if err := s.db.Save(e).Error; err != nil {
				return err
			}
			if err := s.removeCopy(e); err != nil {
				return err
			}
		}
		return s.db.Save(e).Error
	}
	e.WaitFingerprint = ""
	if e.CopyMessageID != 0 {
		if err := s.updateCopy(b, e, source, combined); err != nil {
			return err
		}
		return s.db.Save(e).Error
	}
	// Persist the nonce before attempting the nontransactional Discord create.
	e.SendNonce = strings.ReplaceAll(uuid.NewString(), "-", "")[:25]
	now := time.Now().UTC()
	e.SendStartedAt = &now
	e.CopyChannelID = b.ChannelID
	if err := s.db.Save(e).Error; err != nil {
		return err
	}
	id, err := s.transport.Publish(b, source, combined, e.SendNonce)
	if id != 0 {
		// Creation is confirmed even if adding seed reactions failed.
		e.CopyMessageID = id
		e.SendNonce = ""
		e.SendStartedAt = nil
		return errors.Join(err, s.db.Save(e).Error)
	}
	if errors.Is(err, ErrNotSent) {
		e.SendNonce = ""
		e.SendStartedAt = nil
		e.CopyChannelID = 0
		return errors.Join(err, s.db.Save(e).Error)
	}
	if err != nil {
		return err
	}
	return ErrAmbiguousSend
}

func (s *Service) updateCopy(b model.Starboard, e *model.StarboardEntry, source *discord.Message, v Votes) error {
	err := s.transport.Update(b, source, e.CopyMessageID, v)
	if errors.Is(err, ErrNotFound) {
		e.Suppressed = true
		e.CopyMessageID = 0
		e.CopyChannelID = 0
		return s.db.Save(e).Error
	}
	return err
}
func (s *Service) removeCopy(e *model.StarboardEntry) error {
	if e.CopyMessageID == 0 {
		return nil
	}
	cleanup := model.StarboardCleanup{GuildID: e.GuildID, ChannelID: e.CopyChannelID, MessageID: e.CopyMessageID}
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&cleanup).Error; err != nil {
		return err
	}
	if err := s.transport.Delete(e.CopyChannelID, e.CopyMessageID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	oldID := e.CopyMessageID
	e.CopyMessageID = 0
	e.CopyChannelID = 0
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(e).Error; err != nil {
			return err
		}
		return tx.Where("guild_id = ? AND message_id = ?", e.GuildID, oldID).Delete(&model.StarboardCleanup{}).Error
	})
}

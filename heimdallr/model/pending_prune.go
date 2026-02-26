package model

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemberPendingPrune struct {
	GuildID   snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	PruneID   uuid.UUID    `gorm:"primaryKey;autoIncrement:false"`
	UserID    snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	Timestamp time.Time    `gorm:"autoCreateTime;index"`
	Pruned    bool         `gorm:"default:false"`
}

func AddMembersToBePruned(pruneID uuid.UUID, members []discord.Member) error {
	ctx := context.Background()
	session := DB.Session(&gorm.Session{SkipDefaultTransaction: true})
	err := session.Transaction(
		func(tx *gorm.DB) error {
			for _, member := range members {
				err := gorm.G[MemberPendingPrune](tx).Create(
					ctx, &MemberPendingPrune{
						GuildID: member.GuildID,
						PruneID: pruneID,
						UserID:  member.User.ID,
					},
				)

				if err != nil {
					return err
				}
			}
			return nil
		},
	)

	if err != nil {
		slog.Warn("failed to add members to prune list")
		return err
	}
	return nil
}

func GetMembersToPrune(pruneID uuid.UUID, guildID snowflake.ID) ([]MemberPendingPrune, error) {
	ctx := context.Background()
	session := DB.Session(
		&gorm.Session{SkipDefaultTransaction: true},
	)
	pendingPrunes, err := gorm.G[MemberPendingPrune](session).Where(
		"guild_id = ? AND prune_id = ? AND pruned <> 1", guildID, pruneID,
	).Find(ctx)
	if err != nil {
		return nil, err
	}

	return pendingPrunes, nil
}

func GetPrunedMembers(pruneID uuid.UUID, guildID snowflake.ID) ([]MemberPendingPrune, error) {
	ctx := context.Background()
	session := DB.Session(&gorm.Session{SkipDefaultTransaction: true})
	prunedMembers, err := gorm.G[MemberPendingPrune](session).Where(
		"guild_id = ? AND prune_id = ? AND pruned = 1", guildID, pruneID,
	).Find(ctx)

	return prunedMembers, err
}

func RemovePrunedMembers(guildID snowflake.ID) error {
	ctx := context.Background()
	session := DB.Session(&gorm.Session{SkipDefaultTransaction: true})
	_, err := gorm.G[MemberPendingPrune](session).Where("guild_id = ? AND pruned = 1", guildID).Delete(ctx)
	return err
}

func RemoveMembersByPruneID(pruneID uuid.UUID, guildID snowflake.ID) error {
	ctx := context.Background()
	session := DB.Session(&gorm.Session{SkipDefaultTransaction: true})
	_, err := gorm.G[MemberPendingPrune](session).Where("guild_id = ? AND prune_id = ?", guildID, pruneID).Delete(ctx)
	return err
}

func SetMemberPruned(guildID snowflake.ID, userID snowflake.ID, pruned bool) error {
	ctx := context.Background()
	session := DB.Session(&gorm.Session{SkipDefaultTransaction: true})
	_, err := gorm.G[MemberPendingPrune](session).
		Where("guild_id = ? AND user_id = ?", guildID, userID).
		Select("pruned").
		Updates(ctx, MemberPendingPrune{Pruned: pruned})
	return err
}

func IsMemberPruned(guildID, userID snowflake.ID) (bool, error) {
	ctx := context.Background()
	session := DB.Session(&gorm.Session{SkipDefaultTransaction: true})
	member, err := gorm.G[MemberPendingPrune](session).
		Where("guild_id = ? AND user_id = ?", guildID, userID).
		First(ctx)

	if err != nil {
		return false, err
	}

	return member.Pruned, err
}

func DeletePrunesBeforeTime(t time.Time) error {
	ctx := context.Background()
	_, err := gorm.G[MemberPendingPrune](DB).Where("timestamp < ?", t).Delete(ctx)
	return err
}

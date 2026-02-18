package model

import (
	"context"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

type Infraction struct {
	gorm.Model
	GuildID   snowflake.ID
	UserID    snowflake.ID
	Moderator snowflake.ID
	Reason    string
	Weight    float64
	Timestamp time.Time
	Silent    bool
}

func (i Infraction) Sqid() string {
	id, err := sqidGen.Encode([]uint64{uint64(i.ID)})
	if err != nil {
		panic(err)
	}
	return id

}

func CreateInfraction(guildID, userID, moderator snowflake.ID, reason string, weight float64, silent bool) (*Infraction, error) {
	inf := &Infraction{
		GuildID:   guildID,
		UserID:    userID,
		Moderator: moderator,
		Reason:    reason,
		Weight:    weight,
		Timestamp: time.Now(),
		Silent:    silent,
	}

	res := DB.Create(inf)
	if res.Error != nil {
		return nil, res.Error
	}
	return inf, nil
}

func GetUserInfractions(guildID, userID snowflake.ID, limit, offset int) ([]Infraction, int64, error) {
	var infractions []Infraction
	res := DB.Order("timestamp desc").Where("guild_id = ? AND user_id = ?", guildID, userID).
		Offset(offset).Limit(limit).Find(&infractions)
	if res.Error != nil {
		return nil, 0, res.Error
	}

	var count int64 = 0
	res = DB.Model(&Infraction{}).Where("guild_id = ? AND user_id = ?", guildID, userID).Count(&count)
	if res.Error != nil {
		return nil, 0, res.Error
	}
	return infractions, count, nil
}

var ErrNoSqid = errors.New("no sqid could be decoded")

func DeleteInfractionBySqid(sqid string, guildID snowflake.ID) error {
	ids := sqidGen.Decode(sqid)
	if len(ids) < 1 {
		return ErrNoSqid
	}
	id := uint(ids[0])

	_, res := gorm.G[Infraction](DB).Where("id = ? AND guild_id = ?", id, guildID).Delete(context.Background())
	return res
}

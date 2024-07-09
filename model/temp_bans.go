package model

import (
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm/clause"
	"time"
)

type TempBan struct {
	GuildID snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	UserID  snowflake.ID `gorm:"primaryKey;autoIncrement:false"`

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`

	Reason string
	Banner snowflake.ID
	Until  time.Time
}

func CreateTempBan(guildID, userID, banner snowflake.ID, reason string, until time.Time) (*TempBan, error) {
	tb := &TempBan{
		GuildID: guildID,
		UserID:  userID,
		Banner:  banner,
		Reason:  reason,
		Until:   until,
	}

	res := DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(tb)
	if res.Error != nil {
		return nil, res.Error
	}
	return tb, nil
}

func GetTempBan(guildID, userID snowflake.ID) (*TempBan, error) {
	tb := &TempBan{
		GuildID: guildID,
		UserID:  userID,
	}

	res := DB.First(tb)
	if res.Error != nil {
		return nil, res.Error
	}
	return tb, nil
}

func GetTempBans(guildID snowflake.ID) ([]TempBan, error) {
	var tbs []TempBan
	res := DB.Where("guild_id = ?", guildID).Find(&tbs)
	if res.Error != nil {
		return nil, res.Error
	}
	return tbs, nil
}

func GetExpiredTempBans() ([]TempBan, error) {
	var tbs []TempBan
	res := DB.Where("until < ?", time.Now()).Find(&tbs)
	if res.Error != nil {
		return nil, res.Error
	}
	return tbs, nil
}

func (tb *TempBan) Delete() error {
	res := DB.Delete(tb)
	if res.Error != nil {
		return res.Error
	}
	return nil
}

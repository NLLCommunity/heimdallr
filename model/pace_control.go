package model

import (
	"github.com/disgoorg/snowflake/v2"
)

type PaceControl struct {
	GuildID          snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	ChannelID        snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	Enabled          bool
	TargetWPM        int `gorm:"default:100"`
	MinSlowmode      int `gorm:"default:0"`
	MaxSlowmode      int `gorm:"default:30"`
	ActivationWPM     int `gorm:"default:0"`
	WPMWindowSeconds  int `gorm:"default:60"`
	UserWindowSeconds int `gorm:"default:120"`
}

func GetPaceControlChannels(guildID snowflake.ID) ([]PaceControl, error) {
	var channels []PaceControl
	res := DB.Where("guild_id = ? AND enabled = ?", guildID, true).Find(&channels)
	return channels, res.Error
}

func GetAllPaceControlChannels() ([]PaceControl, error) {
	var channels []PaceControl
	res := DB.Where("enabled = ?", true).Find(&channels)
	return channels, res.Error
}

func GetPaceControl(guildID, channelID snowflake.ID) (*PaceControl, error) {
	var pc PaceControl
	res := DB.Where("guild_id = ? AND channel_id = ?", guildID, channelID).First(&pc)
	if res.Error != nil {
		return nil, res.Error
	}
	return &pc, nil
}

func SetPaceControl(pc *PaceControl) error {
	res := DB.Save(pc)
	return res.Error
}

func DeletePaceControl(guildID, channelID snowflake.ID) error {
	res := DB.Where("guild_id = ? AND channel_id = ?", guildID, channelID).Delete(&PaceControl{})
	return res.Error
}

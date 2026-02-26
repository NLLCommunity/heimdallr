package model

import (
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type ModmailSettings struct {
	GuildID   snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	UpdatedAt time.Time    `gorm:"autoUpdateTime"`

	ReportThreadsChannel      snowflake.ID
	ReportNotificationChannel snowflake.ID
	ReportPingRole            snowflake.ID
}

func GetModmailSettings(guildID snowflake.ID) (*ModmailSettings, error) {
	settings := ModmailSettings{GuildID: guildID}
	res := DB.FirstOrCreate(&settings, "guild_id = ?", guildID)

	if res.Error != nil {
		return nil, res.Error
	}

	return &settings, nil
}

func SetModmailSettings(settings *ModmailSettings) error {
	res := DB.Save(settings)
	return res.Error
}

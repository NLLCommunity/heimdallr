package model

import (
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type GuildSettings struct {
	GuildID   snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	UpdatedAt time.Time    `gorm:"autoUpdateTime"`

	// ModeratorChannel is the channel where notifications and other
	// information for moderators and administrators are sent.
	ModeratorChannel snowflake.ID

	// InfractionHalfLifeDays is the half-life time of infractions in days.
	InfractionHalfLifeDays      float64
	NotifyOnWarnedUserJoin      bool
	NotifyWarnSeverityThreshold float64 `gorm:"default:1.0"`

	GatekeepEnabled              bool
	GatekeepPendingRole          snowflake.ID
	GatekeepApprovedRole         snowflake.ID
	GatekeepAddPendingRoleOnJoin bool
	GatekeepApprovedMessage      string

	JoinMessageEnabled  bool
	JoinMessage         string
	LeaveMessageEnabled bool
	LeaveMessage        string
	JoinLeaveChannel    snowflake.ID
}

func GetGuildSettings(guildID snowflake.ID) (*GuildSettings, error) {
	settings := GuildSettings{GuildID: guildID}
	res := DB.FirstOrCreate(&settings, "guild_id = ?", guildID)
	if res.Error != nil {
		return nil, res.Error
	}
	return &settings, nil
}

func SetGuildSettings(settings *GuildSettings) error {
	res := DB.Save(settings)
	if res.Error != nil {
		return res.Error
	}
	return nil
}

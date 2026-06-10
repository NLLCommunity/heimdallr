package model

import (
	"errors"
	"log/slog"
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

	GatekeepEnabled               bool
	GatekeepPendingRole           snowflake.ID
	GatekeepApprovedRole          snowflake.ID
	GatekeepAddPendingRoleOnJoin  bool
	GatekeepApprovedMessage       string
	GatekeepApprovedMessageV2     bool
	GatekeepApprovedMessageV2Json string

	JoinMessageEnabled  bool
	JoinMessage         string
	JoinMessageV2       bool
	JoinMessageV2Json   string
	LeaveMessageEnabled bool
	LeaveMessage        string
	LeaveMessageV2      bool
	LeaveMessageV2Json  string
	JoinLeaveChannel    snowflake.ID

	AntiSpamEnabled         bool
	AntiSpamCount           int `gorm:"default:5"`
	AntiSpamCooldownSeconds int `gorm:"default:20"`
	AntiSpamTimeoutMinutes  int `gorm:"default:720"` //12 hours

	BanFooter           string
	AlwaysSendBanFooter bool

	// PostsModRoleID grants a single role the ability to manage posts in
	// the web dashboard. Zero means "admins only" — there is no implicit
	// default, so post-mod access requires an admin to opt in by setting
	// the role on the settings page. Replaces the prior reliance on
	// Discord's per-command permission overrides for /post-dashboard.
	PostsModRoleID snowflake.ID

	// AuditLogEnabled is the master per-guild toggle for the bot's audit
	// log. When false, no rows are written for the guild. Disabling it
	// does not prune existing rows — those follow the retention schedule.
	AuditLogEnabled bool

	// AuditMessageRetentionDays / AuditMemberRetentionDays /
	// AuditGuildRetentionDays are per-guild retention overrides in days.
	// nil means "use the bot-operator default from config". A guild may
	// only LOWER retention versus the bot ceiling — values above the bot
	// max are rejected at the settings handler. 0 means "forever" (only
	// permitted if the bot ceiling itself is 0).
	AuditMessageRetentionDays *uint
	AuditMemberRetentionDays  *uint
	AuditGuildRetentionDays   *uint
}

func GetGuildSettings(guildID snowflake.ID) (*GuildSettings, error) {
	cur := time.Now()
	settings := GuildSettings{GuildID: guildID}
	res := DB.FirstOrCreate(&settings, "guild_id = ?", guildID)
	if res.Error != nil {
		return nil, res.Error
	}
	dur := time.Since(cur)
	if dur > time.Second {
		slog.Warn("GetGuildSettings took too long", "guild_id", guildID, "dur", dur)
	} else {
		slog.Debug("GetGuildSettings", "guild_id", guildID, "dur", dur)
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

// ValidatePostsModRole rejects @everyone as the posts-mod role. The
// @everyone role's ID equals the guild's ID, but @everyone is never
// present in member.RoleIDs, so persisting it would silently grant
// access to no one. Every writer of PostsModRoleID (dashboard save
// handler and /admin posts command) must call this before
// SetGuildSettings; the returned message is user-facing. It is not
// enforced inside SetGuildSettings itself because that would also make
// unrelated settings saves fail for a guild with a legacy bad row.
func ValidatePostsModRole(guildID, roleID snowflake.ID) error {
	if roleID != 0 && roleID == guildID {
		return errors.New("@everyone cannot be used as the posts mod role. To grant access to everyone, pick a role that all members have.")
	}
	return nil
}

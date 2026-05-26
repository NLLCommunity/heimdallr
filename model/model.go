package model

import (
	"log/slog"

	"github.com/glebarez/sqlite"
	"github.com/sqids/sqids-go"
	"gorm.io/gorm"
)

var sqidGen *sqids.Sqids

var DB *gorm.DB

func init() {
	var err error
	sqidGen, err = sqids.New(sqids.Options{Alphabet: "abcdefghikmnpqrstuvwxyz1234567890", MinLength: 5})
	if err != nil {
		panic(err)
	}
}

func InitDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path))
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(
		&Infraction{},
		&GuildSettings{},
		&ModmailSettings{},
		&TempBan{},
		&MemberPendingPrune{},
		&DashboardOAuthState{},
		&DashboardSession{},
		&Post{},
		&PostMessage{},
		&AuditLogEntry{},
	)
	if err == nil {
		// Drop the legacy login-code table left over from the magic-link
		// auth flow. Ignored if it never existed (fresh installs).
		_ = db.Migrator().DropTable("dashboard_login_codes")
	}
	if err != nil {
		slog.Error("failed to migrate database", "error", err)
		return nil, err
	}

	// Expression index for the audit log channel filter: message events
	// store the channel as details.channel_id, not target_id, so the
	// "#channel" search path goes through json_extract. Without this
	// index the count(*) on filtered queries scans every row in the
	// guild. json_valid gates json_extract so rows with empty or
	// invalid details (legacy rows, tests that bypass audit.commit)
	// don't break inserts — json_extract errors on non-JSON input, but
	// SQLite's AND is short-circuiting, so json_valid=0 skips the call.
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_details_channel
		ON audit_log_entries (guild_id, json_extract(details, '$.channel_id'))
		WHERE json_valid(details) AND json_extract(details, '$.channel_id') IS NOT NULL`).Error; err != nil {
		slog.Warn("failed to create audit_log channel_id expression index", "error", err)
	}

	DB = db
	return db, nil
}

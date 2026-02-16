package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

// tokenHash returns the hex-encoded SHA-256 of a session token.
// The DB stores only this hash; the raw token lives only in the cookie.
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

const (
	loginCodeExpiry = 5 * time.Minute
	sessionExpiry   = 24 * time.Hour
)

type DashboardLoginCode struct {
	Code      string `gorm:"primaryKey"`
	UserID    snowflake.ID
	Username  string
	Avatar    string
	ExpiresAt time.Time
}

type DashboardSession struct {
	Token     string       `gorm:"primaryKey"`
	UserID    snowflake.ID `gorm:"index"`
	Username  string
	Avatar    string
	ExpiresAt time.Time
}

func CreateLoginCode(userID snowflake.ID, username, avatar string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := hex.EncodeToString(b)

	loginCode := DashboardLoginCode{
		Code:      code,
		UserID:    userID,
		Username:  username,
		Avatar:    avatar,
		ExpiresAt: time.Now().Add(loginCodeExpiry),
	}
	if err := DB.Create(&loginCode).Error; err != nil {
		return "", err
	}
	return code, nil
}

func ExchangeLoginCode(code string) (*DashboardSession, error) {
	var session *DashboardSession

	err := DB.Transaction(func(tx *gorm.DB) error {
		var loginCode DashboardLoginCode
		if err := tx.Where("code = ? AND expires_at > ?", code, time.Now()).First(&loginCode).Error; err != nil {
			return errors.New("invalid or expired login code")
		}

		// Atomically delete the login code and verify we actually deleted it.
		// If RowsAffected == 0, a concurrent request already consumed it.
		result := tx.Delete(&loginCode)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("invalid or expired login code")
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		rawToken := hex.EncodeToString(b)

		dbSession := DashboardSession{
			Token:     tokenHash(rawToken), // store hash, never the raw token
			UserID:    loginCode.UserID,
			Username:  loginCode.Username,
			Avatar:    loginCode.Avatar,
			ExpiresAt: time.Now().Add(sessionExpiry),
		}
		if err := tx.Create(&dbSession).Error; err != nil {
			return err
		}

		// Return the raw token to the caller for use in the session cookie.
		// The DB record holds only the hash.
		session = &DashboardSession{
			Token:     rawToken,
			UserID:    dbSession.UserID,
			Username:  dbSession.Username,
			Avatar:    dbSession.Avatar,
			ExpiresAt: dbSession.ExpiresAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

func GetSession(token string) (*DashboardSession, error) {
	var session DashboardSession
	if err := DB.Where("token = ? AND expires_at > ?", tokenHash(token), time.Now()).First(&session).Error; err != nil {
		return nil, errors.New("invalid or expired session")
	}
	return &session, nil
}

func DeleteSession(token string) error {
	return DB.Where("token = ?", tokenHash(token)).Delete(&DashboardSession{}).Error
}

func CleanExpiredSessions() {
	now := time.Now()
	DB.Where("expires_at <= ?", now).Delete(&DashboardLoginCode{})
	DB.Where("expires_at <= ?", now).Delete(&DashboardSession{})
}

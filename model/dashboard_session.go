package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

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
	var loginCode DashboardLoginCode
	if err := DB.Where("code = ? AND expires_at > ?", code, time.Now()).First(&loginCode).Error; err != nil {
		return nil, errors.New("invalid or expired login code")
	}

	DB.Delete(&loginCode)

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}

	session := &DashboardSession{
		Token:     hex.EncodeToString(b),
		UserID:    loginCode.UserID,
		Username:  loginCode.Username,
		Avatar:    loginCode.Avatar,
		ExpiresAt: time.Now().Add(sessionExpiry),
	}
	if err := DB.Create(session).Error; err != nil {
		return nil, err
	}
	return session, nil
}

func GetSession(token string) (*DashboardSession, error) {
	var session DashboardSession
	if err := DB.Where("token = ? AND expires_at > ?", token, time.Now()).First(&session).Error; err != nil {
		return nil, errors.New("invalid or expired session")
	}
	return &session, nil
}

func DeleteSession(token string) error {
	return DB.Where("token = ?", token).Delete(&DashboardSession{}).Error
}

func CleanExpiredSessions() {
	now := time.Now()
	DB.Where("expires_at <= ?", now).Delete(&DashboardLoginCode{})
	DB.Where("expires_at <= ?", now).Delete(&DashboardSession{})
}

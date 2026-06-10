package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// newRandomToken returns a fresh hex-encoded 256-bit random token. Both
// OAuth state tokens and session tokens are minted through this so their
// entropy and encoding cannot silently diverge.
func newRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

const (
	// oauthStateExpiry must cover the time between /oauth/start issuing
	// the state row and the user completing the Discord consent screen.
	// Discord's authorize page typically completes in seconds, but we
	// leave headroom for users who get prompted to re-auth against
	// Discord itself before the consent step.
	oauthStateExpiry = 10 * time.Minute
	SessionExpiry    = 24 * time.Hour
)

// DashboardOAuthState gates the Discord OAuth redirect against CSRF:
//
//   - The state token is the primary key. /oauth/start generates a fresh
//     random token, stores this row, and includes the token in both the
//     authorize URL and a short-lived browser cookie. /oauth/callback
//     must see both — DB row + matching cookie — before exchanging the
//     code, which prevents login-CSRF (an attacker-crafted authorize
//     URL pasted to a victim's browser).
//   - ReturnTo carries the originally-requested URL across the redirect
//     so a deep-link visit lands the user where they intended after
//     OAuth completes. Empty means "/guilds".
//
// Rows are single-use: ConsumeOAuthState deletes the row atomically.
type DashboardOAuthState struct {
	State     string `gorm:"primaryKey"`
	ReturnTo  string
	ExpiresAt time.Time
}

// DashboardSession stores both the local session record and the user's
// Discord OAuth tokens. AccessTokenEnc / RefreshTokenEnc are AES-256-GCM
// ciphertexts produced by TokenCrypto; the plaintext only exists inside
// the request that fetched it. TokenExpiresAt is the absolute time the
// access token stops working, so handlers can decide whether to refresh
// without round-tripping through Discord first.
type DashboardSession struct {
	Token           string       `gorm:"primaryKey"`
	UserID          snowflake.ID `gorm:"index"`
	Username        string
	Avatar          string
	ExpiresAt       time.Time
	AccessTokenEnc  string
	RefreshTokenEnc string
	TokenExpiresAt  time.Time
}

// SessionIdentity bundles the user info captured from Discord's
// /users/@me response. /oauth/callback builds this from the OAuth user
// after exchanging the code, then hands it to CreateAdminSession.
type SessionIdentity struct {
	UserID   snowflake.ID
	Username string
	Avatar   string
}

// CreateOAuthState records a fresh state token with the optional
// return-to URL and returns the token for use in the authorize URL.
// ConsumeOAuthState validates and deletes the row on the OAuth callback.
func CreateOAuthState(returnTo string) (string, error) {
	state, err := newRandomToken()
	if err != nil {
		return "", err
	}
	row := DashboardOAuthState{
		State:     state,
		ReturnTo:  returnTo,
		ExpiresAt: time.Now().Add(oauthStateExpiry),
	}
	if err := DB.Create(&row).Error; err != nil {
		return "", err
	}
	return state, nil
}

// ConsumeOAuthState atomically validates and deletes the state row.
func ConsumeOAuthState(state string) (*DashboardOAuthState, error) {
	var out *DashboardOAuthState
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row DashboardOAuthState
		if err := tx.Where("state = ? AND expires_at > ?", state, time.Now()).First(&row).Error; err != nil {
			return errors.New("invalid or expired oauth state")
		}
		result := tx.Delete(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("invalid or expired oauth state")
		}
		out = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateAdminSession mints a DashboardSession from the OAuth-verified
// identity and already-sealed tokens. Callers seal the tokens via
// TokenCrypto so this package never touches the encryption key.
//
// Returns the raw cookie token; the DB row stores only its hash.
func CreateAdminSession(id SessionIdentity, sealedAccess, sealedRefresh string, tokenExpiresAt time.Time) (*DashboardSession, error) {
	rawToken, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	dbSession := DashboardSession{
		Token:           tokenHash(rawToken),
		UserID:          id.UserID,
		Username:        id.Username,
		Avatar:          id.Avatar,
		ExpiresAt:       time.Now().Add(SessionExpiry),
		AccessTokenEnc:  sealedAccess,
		RefreshTokenEnc: sealedRefresh,
		TokenExpiresAt:  tokenExpiresAt,
	}
	if err := DB.Create(&dbSession).Error; err != nil {
		return nil, err
	}
	// The caller gets the raw token (for the cookie) in place of the
	// stored hash; every other field matches the row just written.
	dbSession.Token = rawToken
	return &dbSession, nil
}

// UpdateSessionTokens overwrites the encrypted tokens for an existing
// session (used after a successful refresh). The DB row is keyed by the
// stored hash, so callers pass the hashed Token from the in-context
// session — NOT the raw cookie value.
func UpdateSessionTokens(tokenHashed string, sealedAccess, sealedRefresh string, tokenExpiresAt time.Time) error {
	return DB.Model(&DashboardSession{}).
		Where("token = ?", tokenHashed).
		Updates(map[string]any{
			"access_token_enc":  sealedAccess,
			"refresh_token_enc": sealedRefresh,
			"token_expires_at":  tokenExpiresAt,
		}).Error
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

func CleanExpiredSessions() error {
	now := time.Now()
	if err := DB.Where("expires_at <= ?", now).Delete(&DashboardOAuthState{}).Error; err != nil {
		return fmt.Errorf("clean expired oauth states: %w", err)
	}
	if err := DB.Where("expires_at <= ?", now).Delete(&DashboardSession{}).Error; err != nil {
		return fmt.Errorf("clean expired sessions: %w", err)
	}
	return nil
}

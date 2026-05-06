package model

import (
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *ModelTestSuite) TestExchangeLoginCode_Success() {
	userID := snowflake.ID(111222333)
	code, err := CreateLoginCode(userID, "alice", "avatar-hash")
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), code)

	session, err := ExchangeLoginCode(code)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), session)
	assert.Equal(suite.T(), userID, session.UserID)
	assert.Equal(suite.T(), "alice", session.Username)
	assert.Equal(suite.T(), "avatar-hash", session.Avatar)
	assert.NotEmpty(suite.T(), session.Token)
	assert.True(suite.T(), session.ExpiresAt.After(time.Now()))
}

func (suite *ModelTestSuite) TestExchangeLoginCode_IsOneTime() {
	code, err := CreateLoginCode(snowflake.ID(111222333), "alice", "")
	require.NoError(suite.T(), err)

	_, err = ExchangeLoginCode(code)
	require.NoError(suite.T(), err)

	// A login code must be single-use: the second exchange must fail and not
	// produce a session, even if the original session is still valid.
	_, err = ExchangeLoginCode(code)
	assert.Error(suite.T(), err, "login code must be single-use")

	var sessionCount int64
	require.NoError(suite.T(), DB.Model(&DashboardSession{}).Count(&sessionCount).Error)
	assert.Equal(suite.T(), int64(1), sessionCount, "second exchange must not create a second session")
}

func (suite *ModelTestSuite) TestExchangeLoginCode_ExpiredCodeRejected() {
	// Insert a code that's already past its expiry. ExchangeLoginCode's
	// query filters on expires_at > now, so this should not match.
	expired := DashboardLoginCode{
		Code:      "expired-code-fixture",
		UserID:    snowflake.ID(111222333),
		Username:  "alice",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	require.NoError(suite.T(), DB.Create(&expired).Error)

	_, err := ExchangeLoginCode(expired.Code)
	assert.Error(suite.T(), err, "expired login code must be rejected")
}

func (suite *ModelTestSuite) TestExchangeLoginCode_UnknownCodeRejected() {
	_, err := ExchangeLoginCode("does-not-exist")
	assert.Error(suite.T(), err)
}

func (suite *ModelTestSuite) TestGetSession_RequiresRawToken() {
	code, err := CreateLoginCode(snowflake.ID(111222333), "alice", "")
	require.NoError(suite.T(), err)
	session, err := ExchangeLoginCode(code)
	require.NoError(suite.T(), err)

	// The raw token (as it would arrive in the cookie) succeeds.
	got, err := GetSession(session.Token)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), session.UserID, got.UserID)

	// Passing the hash directly must not authenticate. If it did, anyone
	// reading the DB would have working session credentials.
	_, err = GetSession(tokenHash(session.Token))
	assert.Error(suite.T(), err, "DB-stored hash must not be usable as a lookup token")
}

func (suite *ModelTestSuite) TestGetSession_DBStoresHashOnly() {
	code, err := CreateLoginCode(snowflake.ID(111222333), "alice", "")
	require.NoError(suite.T(), err)
	session, err := ExchangeLoginCode(code)
	require.NoError(suite.T(), err)

	var stored DashboardSession
	require.NoError(suite.T(),
		DB.Where("user_id = ?", session.UserID).First(&stored).Error)

	assert.NotEqual(suite.T(), session.Token, stored.Token,
		"raw session token must never appear in the DB")
	assert.Equal(suite.T(), tokenHash(session.Token), stored.Token,
		"DB must store the SHA-256 hash of the raw token")
}

func (suite *ModelTestSuite) TestGetSession_ExpiredSessionRejected() {
	raw := "raw-session-token-fixture"
	expired := DashboardSession{
		Token:     tokenHash(raw),
		UserID:    snowflake.ID(111222333),
		Username:  "alice",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	require.NoError(suite.T(), DB.Create(&expired).Error)

	_, err := GetSession(raw)
	assert.Error(suite.T(), err, "expired session must be rejected")
}

func (suite *ModelTestSuite) TestDeleteSession_RemovesByRawToken() {
	code, err := CreateLoginCode(snowflake.ID(111222333), "alice", "")
	require.NoError(suite.T(), err)
	session, err := ExchangeLoginCode(code)
	require.NoError(suite.T(), err)

	require.NoError(suite.T(), DeleteSession(session.Token))

	_, err = GetSession(session.Token)
	assert.Error(suite.T(), err, "session must be unusable after DeleteSession")
}

func (suite *ModelTestSuite) TestCleanExpiredSessions() {
	// Two expired records and one fresh one across both tables.
	require.NoError(suite.T(), DB.Create(&DashboardLoginCode{
		Code: "expired-code", ExpiresAt: time.Now().Add(-time.Minute),
	}).Error)
	require.NoError(suite.T(), DB.Create(&DashboardLoginCode{
		Code: "fresh-code", ExpiresAt: time.Now().Add(time.Minute),
	}).Error)
	require.NoError(suite.T(), DB.Create(&DashboardSession{
		Token: tokenHash("expired"), ExpiresAt: time.Now().Add(-time.Minute),
	}).Error)
	require.NoError(suite.T(), DB.Create(&DashboardSession{
		Token: tokenHash("fresh"), ExpiresAt: time.Now().Add(time.Minute),
	}).Error)

	require.NoError(suite.T(), CleanExpiredSessions())

	var codeCount, sessionCount int64
	require.NoError(suite.T(), DB.Model(&DashboardLoginCode{}).Count(&codeCount).Error)
	require.NoError(suite.T(), DB.Model(&DashboardSession{}).Count(&sessionCount).Error)
	assert.Equal(suite.T(), int64(1), codeCount, "expired login code must be deleted")
	assert.Equal(suite.T(), int64(1), sessionCount, "expired session must be deleted")
}

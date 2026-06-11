package model

import (
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *ModelTestSuite) TestOAuthState_RoundTrip() {
	state, err := CreateOAuthState("/guild/12345")
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), state)

	got, err := ConsumeOAuthState(state)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), got)
	assert.Equal(suite.T(), "/guild/12345", got.ReturnTo)

	// Single-use: second consume must fail.
	_, err = ConsumeOAuthState(state)
	assert.Error(suite.T(), err, "oauth state must be single-use")
}

func (suite *ModelTestSuite) TestOAuthState_EmptyReturnToRoundTrip() {
	// A bare /oauth/start without return_to stores "" — must still be
	// usable on the callback (the handler defaults to /guilds).
	state, err := CreateOAuthState("")
	require.NoError(suite.T(), err)
	got, err := ConsumeOAuthState(state)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "", got.ReturnTo)
}

func (suite *ModelTestSuite) TestOAuthState_ExpiredRejected() {
	expired := DashboardOAuthState{
		State:     "expired-state-fixture",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	require.NoError(suite.T(), DB.Create(&expired).Error)

	_, err := ConsumeOAuthState(expired.State)
	assert.Error(suite.T(), err, "expired oauth state must be rejected")
}

func (suite *ModelTestSuite) TestOAuthState_UnknownRejected() {
	_, err := ConsumeOAuthState("does-not-exist")
	assert.Error(suite.T(), err)
}

func (suite *ModelTestSuite) TestCreateAdminSession() {
	identity := SessionIdentity{
		UserID:   snowflake.ID(111222333),
		Username: "alice",
		Avatar:   "avatar-hash",
	}
	expires := time.Now().Add(time.Hour)
	session, err := CreateAdminSession(identity, "sealed-access", "sealed-refresh", expires)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), session)
	assert.Equal(suite.T(), identity.UserID, session.UserID)
	assert.NotEmpty(suite.T(), session.Token, "raw cookie token must be returned")
	assert.Equal(suite.T(), "sealed-access", session.AccessTokenEnc)
	assert.Equal(suite.T(), "sealed-refresh", session.RefreshTokenEnc)
}

func (suite *ModelTestSuite) TestUpdateSessionTokens() {
	identity := SessionIdentity{UserID: snowflake.ID(111222333), Username: "alice"}
	session, err := CreateAdminSession(identity, "old-access", "old-refresh", time.Now().Add(time.Minute))
	require.NoError(suite.T(), err)

	newExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	require.NoError(suite.T(),
		UpdateSessionTokens(tokenHash(session.Token), "new-access", "new-refresh", newExpiry))

	var stored DashboardSession
	require.NoError(suite.T(),
		DB.Where("token = ?", tokenHash(session.Token)).First(&stored).Error)
	assert.Equal(suite.T(), "new-access", stored.AccessTokenEnc)
	assert.Equal(suite.T(), "new-refresh", stored.RefreshTokenEnc)
	assert.True(suite.T(),
		stored.TokenExpiresAt.Truncate(time.Second).Equal(newExpiry),
		"TokenExpiresAt must round-trip through Update")
}

func (suite *ModelTestSuite) TestGetSession_RequiresRawToken() {
	identity := SessionIdentity{UserID: snowflake.ID(111222333), Username: "alice"}
	session, err := CreateAdminSession(identity, "", "", time.Time{})
	require.NoError(suite.T(), err)

	got, err := GetSession(session.Token)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), session.UserID, got.UserID)

	// Passing the hash directly must not authenticate. If it did, anyone
	// reading the DB would have working session credentials.
	_, err = GetSession(tokenHash(session.Token))
	assert.Error(suite.T(), err, "DB-stored hash must not be usable as a lookup token")
}

func (suite *ModelTestSuite) TestGetSession_DBStoresHashOnly() {
	identity := SessionIdentity{UserID: snowflake.ID(111222333), Username: "alice"}
	session, err := CreateAdminSession(identity, "", "", time.Time{})
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
	identity := SessionIdentity{UserID: snowflake.ID(111222333), Username: "alice"}
	session, err := CreateAdminSession(identity, "", "", time.Time{})
	require.NoError(suite.T(), err)

	require.NoError(suite.T(), DeleteSession(session.Token))

	_, err = GetSession(session.Token)
	assert.Error(suite.T(), err, "session must be unusable after DeleteSession")
}

func (suite *ModelTestSuite) TestCleanExpiredSessions() {
	require.NoError(suite.T(), DB.Create(&DashboardOAuthState{
		State: "expired-state", ExpiresAt: time.Now().Add(-time.Minute),
	}).Error)
	require.NoError(suite.T(), DB.Create(&DashboardOAuthState{
		State: "fresh-state", ExpiresAt: time.Now().Add(time.Minute),
	}).Error)
	require.NoError(suite.T(), DB.Create(&DashboardSession{
		Token: tokenHash("expired"), ExpiresAt: time.Now().Add(-time.Minute),
	}).Error)
	require.NoError(suite.T(), DB.Create(&DashboardSession{
		Token: tokenHash("fresh"), ExpiresAt: time.Now().Add(time.Minute),
	}).Error)

	require.NoError(suite.T(), CleanExpiredSessions())

	var stateCount, sessionCount int64
	require.NoError(suite.T(), DB.Model(&DashboardOAuthState{}).Count(&stateCount).Error)
	require.NoError(suite.T(), DB.Model(&DashboardSession{}).Count(&sessionCount).Error)
	assert.Equal(suite.T(), int64(1), stateCount, "expired oauth state must be deleted")
	assert.Equal(suite.T(), int64(1), sessionCount, "expired session must be deleted")
}

package model

import (
	"os"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// ModelTestSuite provides a test suite with database setup/teardown.
type ModelTestSuite struct {
	suite.Suite
	db       *gorm.DB
	tempFile string
}

func (suite *ModelTestSuite) SetupSuite() {
	// Create a temporary database file.
	tempFile, err := os.CreateTemp("", "heimdallr_test_*.db")
	require.NoError(suite.T(), err)
	suite.tempFile = tempFile.Name()
	tempFile.Close()

	// Initialize test database.
	suite.db, err = InitDB(suite.tempFile)
	require.NoError(suite.T(), err)
}

func (suite *ModelTestSuite) TearDownSuite() {
	// Clean up.
	if suite.db != nil {
		sqlDB, err := suite.db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
	os.Remove(suite.tempFile)
}

func (suite *ModelTestSuite) SetupTest() {
	// Clean all tables before each test.
	suite.db.Exec("DELETE FROM infractions")
	suite.db.Exec("DELETE FROM guild_settings")
	suite.db.Exec("DELETE FROM modmail_settings")
	suite.db.Exec("DELETE FROM temp_bans")
}

func TestModelSuite(t *testing.T) {
	suite.Run(t, new(ModelTestSuite))
}

func (suite *ModelTestSuite) TestInitDB() {
	// Test that database initialization works with a separate database file.
	tempFile, err := os.CreateTemp("", "heimdallr_init_test_*.db")
	require.NoError(suite.T(), err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Save the current global DB.
	originalDB := DB

	db, err := InitDB(tempFile.Name())
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), db)
	assert.Equal(suite.T(), db, DB) // Global DB should be set.

	// Restore the original DB for other tests.
	DB = originalDB

	sqlDB, err := db.DB()
	require.NoError(suite.T(), err)
	defer sqlDB.Close()
}

func (suite *ModelTestSuite) TestCreateInfraction() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	moderator := snowflake.ID(555666777)
	reason := "Test infraction"
	weight := 1.5
	silent := false

	infraction, err := CreateInfraction(guildID, userID, moderator, reason, weight, silent)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), infraction)
	assert.Equal(suite.T(), guildID, infraction.GuildID)
	assert.Equal(suite.T(), userID, infraction.UserID)
	assert.Equal(suite.T(), moderator, infraction.Moderator)
	assert.Equal(suite.T(), reason, infraction.Reason)
	assert.Equal(suite.T(), weight, infraction.Weight)
	assert.Equal(suite.T(), silent, infraction.Silent)
	assert.NotZero(suite.T(), infraction.ID)
	assert.False(suite.T(), infraction.Timestamp.IsZero())
}

func (suite *ModelTestSuite) TestInfractionSqid() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	moderator := snowflake.ID(555666777)

	infraction, err := CreateInfraction(guildID, userID, moderator, "Test", 1.0, false)
	require.NoError(suite.T(), err)

	sqid := infraction.Sqid()
	assert.NotEmpty(suite.T(), sqid)
	assert.GreaterOrEqual(suite.T(), len(sqid), 5) // MinLength is 5
}

func (suite *ModelTestSuite) TestGetUserInfractions() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	moderator := snowflake.ID(555666777)

	// Create multiple infractions
	for i := 0; i < 3; i++ {
		_, err := CreateInfraction(guildID, userID, moderator, "Test infraction", 1.0, false)
		require.NoError(suite.T(), err)
		time.Sleep(time.Millisecond) // Ensure different timestamps.
	}

	// Create infraction for different user to ensure filtering works.
	differentUser := snowflake.ID(111222333)
	_, err := CreateInfraction(guildID, differentUser, moderator, "Different user", 1.0, false)
	require.NoError(suite.T(), err)

	infractions, count, err := GetUserInfractions(guildID, userID, 10, 0)

	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), infractions, 3)
	assert.Equal(suite.T(), int64(3), count)

	// Test that they're ordered by timestamp desc.
	for i := 1; i < len(infractions); i++ {
		assert.True(suite.T(), infractions[i-1].Timestamp.After(infractions[i].Timestamp) ||
			infractions[i-1].Timestamp.Equal(infractions[i].Timestamp))
	}

	// Test pagination.
	infractions, _, err = GetUserInfractions(guildID, userID, 2, 0)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), infractions, 2)

	infractions, _, err = GetUserInfractions(guildID, userID, 2, 2)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), infractions, 1)
}

func (suite *ModelTestSuite) TestDeleteInfractionBySqid() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	moderator := snowflake.ID(555666777)

	infraction, err := CreateInfraction(guildID, userID, moderator, "Test", 1.0, false)
	require.NoError(suite.T(), err)

	sqid := infraction.Sqid()

	// Delete by sqid.
	err = DeleteInfractionBySqid(sqid)
	assert.NoError(suite.T(), err)

	// Verify it's deleted.
	infractions, count, err := GetUserInfractions(guildID, userID, 10, 0)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), infractions, 0)
	assert.Equal(suite.T(), int64(0), count)

	// Test deleting non-existent sqid.
	err = DeleteInfractionBySqid("nonexistent")
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrNoSqid, err)
}

func (suite *ModelTestSuite) TestGetGuildSettings() {
	guildID := snowflake.ID(123456789)

	// First call should create settings.
	settings, err := GetGuildSettings(guildID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), settings)
	assert.Equal(suite.T(), guildID, settings.GuildID)

	// Second call should return existing settings.
	settings2, err := GetGuildSettings(guildID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), settings.GuildID, settings2.GuildID)
}

func (suite *ModelTestSuite) TestSetGuildSettings() {
	guildID := snowflake.ID(123456789)
	modChannel := snowflake.ID(444555666)

	settings, err := GetGuildSettings(guildID)
	require.NoError(suite.T(), err)

	// Modify settings.
	settings.ModeratorChannel = modChannel
	settings.InfractionHalfLifeDays = 90.0
	settings.NotifyOnWarnedUserJoin = true

	err = SetGuildSettings(settings)
	assert.NoError(suite.T(), err)

	// Retrieve and verify changes.
	retrievedSettings, err := GetGuildSettings(guildID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), modChannel, retrievedSettings.ModeratorChannel)
	assert.Equal(suite.T(), 90.0, retrievedSettings.InfractionHalfLifeDays)
	assert.True(suite.T(), retrievedSettings.NotifyOnWarnedUserJoin)
}

func (suite *ModelTestSuite) TestCreateTempBan() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	banner := snowflake.ID(555666777)
	reason := "Temporary ban test"
	until := time.Now().Add(24 * time.Hour)

	tempBan, err := CreateTempBan(guildID, userID, banner, reason, until)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), tempBan)
	assert.Equal(suite.T(), guildID, tempBan.GuildID)
	assert.Equal(suite.T(), userID, tempBan.UserID)
	assert.Equal(suite.T(), banner, tempBan.Banner)
	assert.Equal(suite.T(), reason, tempBan.Reason)
	assert.True(suite.T(), until.Equal(tempBan.Until))
}

func (suite *ModelTestSuite) TestGetTempBan() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	banner := snowflake.ID(555666777)
	until := time.Now().Add(24 * time.Hour)

	// Create temp ban.
	created, err := CreateTempBan(guildID, userID, banner, "Test", until)
	require.NoError(suite.T(), err)

	// Retrieve temp ban.
	retrieved, err := GetTempBan(guildID, userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), created.GuildID, retrieved.GuildID)
	assert.Equal(suite.T(), created.UserID, retrieved.UserID)
	assert.Equal(suite.T(), created.Banner, retrieved.Banner)
	assert.Equal(suite.T(), created.Reason, retrieved.Reason)

	// Test getting non-existent temp ban.
	_, err = GetTempBan(snowflake.ID(999999999), userID)
	assert.Error(suite.T(), err)
}

func (suite *ModelTestSuite) TestGetTempBans() {
	guildID := snowflake.ID(123456789)
	banner := snowflake.ID(555666777)
	until := time.Now().Add(24 * time.Hour)

	// Create multiple temp bans.
	userIDs := []snowflake.ID{987654321, 987654322, 987654323}
	for _, userID := range userIDs {
		_, err := CreateTempBan(guildID, userID, banner, "Test", until)
		require.NoError(suite.T(), err)
	}

	// Create temp ban for different guild.
	differentGuild := snowflake.ID(999888777)
	_, err := CreateTempBan(differentGuild, snowflake.ID(111222333), banner, "Different guild", until)
	require.NoError(suite.T(), err)

	// Get temp bans for specific guild.
	tempBans, err := GetTempBans(guildID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), tempBans, 3)

	for _, tb := range tempBans {
		assert.Equal(suite.T(), guildID, tb.GuildID)
	}
}

func (suite *ModelTestSuite) TestGetExpiredTempBans() {
	guildID := snowflake.ID(123456789)
	banner := snowflake.ID(555666777)

	// Create expired temp ban.
	expiredUntil := time.Now().Add(-time.Hour)
	_, err := CreateTempBan(guildID, snowflake.ID(111), banner, "Expired", expiredUntil)
	require.NoError(suite.T(), err)

	// Create non-expired temp ban.
	futureUntil := time.Now().Add(time.Hour)
	_, err = CreateTempBan(guildID, snowflake.ID(222), banner, "Future", futureUntil)
	require.NoError(suite.T(), err)

	// Get expired temp bans.
	expired, err := GetExpiredTempBans()
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), expired, 1)
	assert.Equal(suite.T(), snowflake.ID(111), expired[0].UserID)
}

func (suite *ModelTestSuite) TestTempBanDelete() {
	guildID := snowflake.ID(123456789)
	userID := snowflake.ID(987654321)
	banner := snowflake.ID(555666777)
	until := time.Now().Add(24 * time.Hour)

	tempBan, err := CreateTempBan(guildID, userID, banner, "Test", until)
	require.NoError(suite.T(), err)

	// Delete temp ban.
	err = tempBan.Delete()
	assert.NoError(suite.T(), err)

	// Verify it's deleted.
	_, err = GetTempBan(guildID, userID)
	assert.Error(suite.T(), err)
}

func (suite *ModelTestSuite) TestGetModmailSettings() {
	guildID := snowflake.ID(123456789)

	// First call should create settings.
	settings, err := GetModmailSettings(guildID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), settings)
	assert.Equal(suite.T(), guildID, settings.GuildID)

	// Second call should return existing settings.
	settings2, err := GetModmailSettings(guildID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), settings.GuildID, settings2.GuildID)
}

func (suite *ModelTestSuite) TestSetModmailSettings() {
	guildID := snowflake.ID(123456789)
	reportChannel := snowflake.ID(444555666)
	notifChannel := snowflake.ID(777888999)
	pingRole := snowflake.ID(111222333)

	settings, err := GetModmailSettings(guildID)
	require.NoError(suite.T(), err)

	// Modify settings.
	settings.ReportThreadsChannel = reportChannel
	settings.ReportNotificationChannel = notifChannel
	settings.ReportPingRole = pingRole

	err = SetModmailSettings(settings)
	assert.NoError(suite.T(), err)

	// Retrieve and verify changes.
	retrievedSettings, err := GetModmailSettings(guildID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), reportChannel, retrievedSettings.ReportThreadsChannel)
	assert.Equal(suite.T(), notifChannel, retrievedSettings.ReportNotificationChannel)
	assert.Equal(suite.T(), pingRole, retrievedSettings.ReportPingRole)
}

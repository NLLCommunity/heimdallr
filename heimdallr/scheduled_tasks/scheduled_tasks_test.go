package scheduled_tasks

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/NLLCommunity/heimdallr/model"
)

// ScheduledTasksTestSuite provides a test suite with database setup/teardown.
type ScheduledTasksTestSuite struct {
	suite.Suite
	tempFile string
}

func (suite *ScheduledTasksTestSuite) SetupSuite() {
	// Create a temporary database file.
	tempFile, err := os.CreateTemp("", "heimdallr_scheduled_test_*.db")
	require.NoError(suite.T(), err)
	suite.tempFile = tempFile.Name()
	tempFile.Close()

	// Initialize test database.
	_, err = model.InitDB(suite.tempFile)
	require.NoError(suite.T(), err)
}

func (suite *ScheduledTasksTestSuite) TearDownSuite() {
	// Clean up.
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
	os.Remove(suite.tempFile)
}

func (suite *ScheduledTasksTestSuite) SetupTest() {
	// Clean all tables before each test.
	model.DB.Exec("DELETE FROM temp_bans")
}

func TestScheduledTasksSuite(t *testing.T) {
	suite.Run(t, new(ScheduledTasksTestSuite))
}

func (suite *ScheduledTasksTestSuite) TestRemoveTempBansWithNoClient() {
	// Test that function handles missing client gracefully.
	ctx := context.Background() // No client in context.

	// This should not panic and should return early.
	assert.NotPanics(suite.T(), func() {
		removeTempBans(ctx)
	})
}

func (suite *ScheduledTasksTestSuite) TestGetExpiredTempBansFlow() {
	// Create expired temp bans.
	guildID := snowflake.ID(123456789)
	userID1 := snowflake.ID(111111111)
	userID2 := snowflake.ID(222222222)
	banner := snowflake.ID(333333333)

	expiredTime := time.Now().Add(-time.Hour)

	_, err := model.CreateTempBan(guildID, userID1, banner, "Test ban 1", expiredTime)
	require.NoError(suite.T(), err)

	_, err = model.CreateTempBan(guildID, userID2, banner, "Test ban 2", expiredTime)
	require.NoError(suite.T(), err)

	// Create non-expired temp ban.
	futureTime := time.Now().Add(time.Hour)
	_, err = model.CreateTempBan(guildID, snowflake.ID(333333333), banner, "Future ban", futureTime)
	require.NoError(suite.T(), err)

	// Test that GetExpiredTempBans returns only expired bans.
	expiredBans, err := model.GetExpiredTempBans()
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), expiredBans, 2)

	// Verify that all returned bans are actually expired.
	for _, ban := range expiredBans {
		assert.True(suite.T(), ban.Until.Before(time.Now()))
	}

	// Test that we can delete expired bans.
	for _, ban := range expiredBans {
		err := ban.Delete()
		assert.NoError(suite.T(), err)
	}

	// Verify that only non-expired bans remain.
	remainingBans, err := model.GetTempBans(guildID)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), remainingBans, 1)
	assert.Equal(suite.T(), snowflake.ID(333333333), remainingBans[0].UserID)
}

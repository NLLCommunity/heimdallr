package model

import (
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBirthdayValidation(t *testing.T) {
	validLeapYear := 2000
	nonLeapYear := 2001
	tinyYear := 999
	futureYear := 2027

	tests := []struct {
		name      string
		day       int
		month     time.Month
		birthYear *int
		policy    LeapDayPolicy
		wantError bool
	}{
		{name: "ordinary date", day: 26, month: time.August},
		{name: "invalid day for month", day: 31, month: time.April, wantError: true},
		{name: "leap day without year", day: 29, month: time.February, policy: LeapDayFebruary28},
		{name: "leap day with leap birth year", day: 29, month: time.February, birthYear: &validLeapYear, policy: LeapDayMarch1},
		{name: "leap day with non-leap birth year", day: 29, month: time.February, birthYear: &nonLeapYear, policy: LeapDayFebruary28, wantError: true},
		{name: "year must have four digits", day: 1, month: time.January, birthYear: &tinyYear, wantError: true},
		{name: "year cannot be in the future", day: 1, month: time.January, birthYear: &futureYear, wantError: true},
		{name: "leap day requires policy", day: 29, month: time.February, wantError: true},
		{name: "leap day rejects unknown policy", day: 29, month: time.February, policy: LeapDayPolicy("unknown"), wantError: true},
		{name: "ordinary date rejects leap policy", day: 1, month: time.March, policy: LeapDayMarch1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBirthday(tt.day, tt.month, tt.birthYear, tt.policy, 2026)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestBirthdayObservedOnLeapDayPolicies(t *testing.T) {
	birthday := Birthday{Day: 29, Month: time.February, LeapDayPolicy: LeapDayFebruary28}

	assert.True(t, BirthdayObservedOn(birthday, time.Date(2027, time.February, 28, 10, 0, 0, 0, time.UTC)))
	assert.False(t, BirthdayObservedOn(birthday, time.Date(2027, time.March, 1, 10, 0, 0, 0, time.UTC)))

	birthday.LeapDayPolicy = LeapDayMarch1
	assert.False(t, BirthdayObservedOn(birthday, time.Date(2027, time.February, 28, 10, 0, 0, 0, time.UTC)))
	assert.True(t, BirthdayObservedOn(birthday, time.Date(2027, time.March, 1, 10, 0, 0, 0, time.UTC)))
	assert.True(t, BirthdayObservedOn(birthday, time.Date(2028, time.February, 29, 10, 0, 0, 0, time.UTC)))
}

func TestBirthdayObservedOnOrdinaryDate(t *testing.T) {
	birthday := Birthday{Day: 26, Month: time.August}

	assert.True(t, BirthdayObservedOn(birthday, time.Date(2026, time.August, 26, 23, 59, 0, 0, time.UTC)))
	assert.False(t, BirthdayObservedOn(birthday, time.Date(2026, time.August, 25, 23, 59, 0, 0, time.UTC)))
}

func (suite *ModelTestSuite) TestBirthdayMigration() {
	assert.True(suite.T(), suite.db.Migrator().HasTable(&Birthday{}))
}

func (suite *ModelTestSuite) TestBirthdayUpsertRetainsAnnouncementMarker() {
	birthYear := 1990
	birthday, err := SetBirthday(10, 20, 7, time.August, &birthYear, "", 2026)
	require.NoError(suite.T(), err)
	require.NoError(suite.T(), MarkBirthdayAnnounced(birthday.GuildID, birthday.UserID, 2026))

	_, err = SetBirthday(10, 20, 8, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	got, err := GetBirthday(10, 20)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 8, got.Day)
	assert.Nil(suite.T(), got.BirthYear)
	assert.Equal(suite.T(), 2026, got.LastAnnouncedYear)
}

func (suite *ModelTestSuite) TestBirthdayCRUDIsScopedToGuildAndUser() {
	_, err := SetBirthday(10, 20, 7, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	_, err = SetBirthday(11, 20, 8, time.September, nil, "", 2026)
	require.NoError(suite.T(), err)
	_, err = SetBirthday(10, 21, 9, time.October, nil, "", 2026)
	require.NoError(suite.T(), err)

	require.NoError(suite.T(), DeleteBirthday(10, 20))
	_, err = GetBirthday(10, 20)
	assert.ErrorIs(suite.T(), err, gorm.ErrRecordNotFound)

	otherGuild, err := GetBirthday(11, 20)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 8, otherGuild.Day)
	otherUser, err := GetBirthday(10, 21)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 9, otherUser.Day)

	assert.NoError(suite.T(), DeleteBirthday(999, 999))
}

func (suite *ModelTestSuite) TestDueBirthdaysSelectsOrdinaryDateAndExcludesAnnouncedRows() {
	for _, userID := range []snowflake.ID{20, 21} {
		_, err := SetBirthday(10, userID, 26, time.August, nil, "", 2026)
		require.NoError(suite.T(), err)
	}
	require.NoError(suite.T(), MarkBirthdayAnnounced(10, 21, 2026))
	_, err := SetBirthday(11, 22, 26, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)

	due, err := DueBirthdays(10, time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC))
	require.NoError(suite.T(), err)
	require.Len(suite.T(), due, 1)
	assert.Equal(suite.T(), snowflake.ID(20), due[0].UserID)
}

func (suite *ModelTestSuite) TestDueBirthdaysAppliesLeapDayFallbackPolicy() {
	_, err := SetBirthday(10, 20, 29, time.February, nil, LeapDayFebruary28, 2027)
	require.NoError(suite.T(), err)
	_, err = SetBirthday(10, 21, 29, time.February, nil, LeapDayMarch1, 2027)
	require.NoError(suite.T(), err)
	_, err = SetBirthday(10, 22, 28, time.February, nil, "", 2027)
	require.NoError(suite.T(), err)
	_, err = SetBirthday(10, 23, 1, time.March, nil, "", 2027)
	require.NoError(suite.T(), err)

	februaryDue, err := DueBirthdays(10, time.Date(2027, time.February, 28, 10, 0, 0, 0, time.UTC))
	require.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), []snowflake.ID{20, 22}, birthdayUserIDs(februaryDue))

	marchDue, err := DueBirthdays(10, time.Date(2027, time.March, 1, 10, 0, 0, 0, time.UTC))
	require.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), []snowflake.ID{21, 23}, birthdayUserIDs(marchDue))

	leapYearDue, err := DueBirthdays(10, time.Date(2028, time.February, 29, 10, 0, 0, 0, time.UTC))
	require.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), []snowflake.ID{20, 21}, birthdayUserIDs(leapYearDue))
}

func birthdayUserIDs(birthdays []Birthday) []snowflake.ID {
	ids := make([]snowflake.ID, 0, len(birthdays))
	for _, birthday := range birthdays {
		ids = append(ids, birthday.UserID)
	}
	return ids
}

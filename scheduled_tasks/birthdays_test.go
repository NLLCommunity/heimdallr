package scheduled_tasks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func (suite *ScheduledTasksTestSuite) TestBirthdayAnnouncementsRespectLocalTimeAndCatchUp() {
	settings := suite.birthdaySettings(10, "Europe/Oslo", 10, 0)
	_, err := model.SetBirthday(10, 20, 26, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	sender := newRecordingBirthdaySender()

	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 7, 59, 0, 0, time.UTC), sender)
	assert.Empty(suite.T(), sender.successful)
	birthday, err := model.GetBirthday(10, 20)
	require.NoError(suite.T(), err)
	assert.Zero(suite.T(), birthday.LastAnnouncedYear)

	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC), sender)
	birthday, err = model.GetBirthday(10, 20)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2026, birthday.LastAnnouncedYear)

	_, err = model.SetBirthday(10, 21, 26, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 21, 45, 0, 0, time.UTC), sender)
	lateBirthday, err := model.GetBirthday(10, 21)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2026, lateBirthday.LastAnnouncedYear)

	assert.Equal(suite.T(), "Europe/Oslo", settings.BirthdayTimezone)
}

func (suite *ScheduledTasksTestSuite) TestBirthdayAnnouncementsUseGuildLocalDate() {
	suite.birthdaySettings(11, "America/New_York", 10, 0)
	_, err := model.SetBirthday(11, 30, 25, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	sender := newRecordingBirthdaySender()

	// 02:00 UTC on August 26 is 22:00 on August 25 in New York.
	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 2, 0, 0, 0, time.UTC), sender)
	birthday, err := model.GetBirthday(11, 30)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2026, birthday.LastAnnouncedYear)
}

func (suite *ScheduledTasksTestSuite) TestBirthdayAnnouncementsRetryFailuresAndContinueOtherUsers() {
	suite.birthdaySettings(10, "UTC", 10, 0)
	_, err := model.SetBirthday(10, 20, 26, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	_, err = model.SetBirthday(10, 21, 26, time.August, nil, "", 2026)
	require.NoError(suite.T(), err)
	sender := newRecordingBirthdaySender()
	sender.failures[20] = 1

	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC), sender)
	failed, err := model.GetBirthday(10, 20)
	require.NoError(suite.T(), err)
	assert.Zero(suite.T(), failed.LastAnnouncedYear)
	succeeded, err := model.GetBirthday(10, 21)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2026, succeeded.LastAnnouncedYear)

	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 10, 15, 0, 0, time.UTC), sender)
	failed, err = model.GetBirthday(10, 20)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2026, failed.LastAnnouncedYear)
	successCount := len(sender.successful)

	runBirthdayAnnouncements(context.Background(), time.Date(2026, 8, 26, 10, 30, 0, 0, time.UTC), sender)
	assert.Len(suite.T(), sender.successful, successCount)
}

func (suite *ScheduledTasksTestSuite) TestBirthdayAnnouncementsHandleDSTSkippedAndRepeatedTimes() {
	suite.birthdaySettings(10, "Europe/Oslo", 2, 30)
	_, err := model.SetBirthday(10, 20, 29, time.March, nil, "", 2026)
	require.NoError(suite.T(), err)
	sender := newRecordingBirthdaySender()

	// 02:30 does not exist on this spring-forward date; 01:00 UTC is 03:00 local.
	runBirthdayAnnouncements(context.Background(), time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC), sender)
	spring, err := model.GetBirthday(10, 20)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2026, spring.LastAnnouncedYear)

	_, err = model.SetBirthday(10, 21, 25, time.October, nil, "", 2026)
	require.NoError(suite.T(), err)
	runBirthdayAnnouncements(context.Background(), time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC), sender)
	countAfterFirstOccurrence := len(sender.successful)
	runBirthdayAnnouncements(context.Background(), time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), sender)
	assert.Len(suite.T(), sender.successful, countAfterFirstOccurrence)
}

func TestBuildBirthdayMessageRendersPlainAndV2WithSafeMentions(t *testing.T) {
	data := utils.MessageTemplateData{
		User:   utils.TemplateUserData{Mention: "<@20>"},
		Age:    "36",
		HasAge: true,
	}
	settings := &model.GuildSettings{
		BirthdayMessage: "Happy {{User.Mention}}{{#HasAge}} — {{Age}}{{/HasAge}}",
	}

	plain, err := buildBirthdayMessage(settings, data, nil, 20)
	require.NoError(t, err)
	assert.Equal(t, "Happy <@20> — 36", plain.Content)
	require.NotNil(t, plain.AllowedMentions)
	assert.Equal(t, []snowflake.ID{20}, plain.AllowedMentions.Users)
	assert.Empty(t, plain.AllowedMentions.Roles)
	assert.Empty(t, plain.AllowedMentions.Parse)

	data.Age = ""
	data.HasAge = false
	plain, err = buildBirthdayMessage(settings, data, nil, 20)
	require.NoError(t, err)
	assert.Equal(t, "Happy <@20>", plain.Content)

	settings.BirthdayMessageV2 = true
	settings.BirthdayMessageV2Json = `[{"type":10,"content":"Happy {{User.Mention}}{{#HasAge}} — {{Age}}{{/HasAge}}"}]`
	data.Age, data.HasAge = "36", true
	v2, err := buildBirthdayMessage(settings, data, nil, 20)
	require.NoError(t, err)
	assert.True(t, v2.Flags.Has(discord.MessageFlagIsComponentsV2))
	encoded, err := json.Marshal(v2.Components)
	require.NoError(t, err)
	var rendered []map[string]any
	require.NoError(t, json.Unmarshal(encoded, &rendered))
	require.Len(t, rendered, 1)
	assert.Equal(t, "Happy <@20> — 36", rendered[0]["content"])
	assert.Equal(t, []snowflake.ID{20}, v2.AllowedMentions.Users)
}

func TestBirthdayAgeOmitsFutureBirthYear(t *testing.T) {
	birthYear := 2027
	age, hasAge := birthdayAge(&birthYear, 2026)

	assert.Empty(t, age)
	assert.False(t, hasAge)
}

func (suite *ScheduledTasksTestSuite) birthdaySettings(
	guildID snowflake.ID,
	timezone string,
	hour int,
	minute int,
) *model.GuildSettings {
	settings, err := model.GetGuildSettings(guildID)
	require.NoError(suite.T(), err)
	settings.BirthdayEnabled = true
	settings.BirthdayChannel = guildID + 1000
	settings.BirthdayTimezone = timezone
	settings.BirthdayHour = hour
	settings.BirthdayMinute = minute
	require.NoError(suite.T(), model.SetGuildSettings(settings))
	return settings
}

type recordingBirthdaySender struct {
	failures   map[snowflake.ID]int
	successful []snowflake.ID
}

func newRecordingBirthdaySender() *recordingBirthdaySender {
	return &recordingBirthdaySender{failures: make(map[snowflake.ID]int)}
}

func (s *recordingBirthdaySender) Send(
	_ context.Context,
	_ *model.GuildSettings,
	birthday model.Birthday,
	_ int,
) error {
	if s.failures[birthday.UserID] > 0 {
		s.failures[birthday.UserID]--
		return errors.New("temporary send failure")
	}
	s.successful = append(s.successful, birthday.UserID)
	return nil
}

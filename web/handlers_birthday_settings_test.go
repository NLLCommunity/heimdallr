package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

func TestSettingsBirthdayRendersScheduleAndMessageControls(t *testing.T) {
	data := partials.BirthdayData{
		GuildID:      "10",
		Enabled:      true,
		Channel:      "100",
		Channels:     []components.ChannelGroup{{Channels: []components.ChannelInfo{{ID: "100", Name: "birthdays", Type: discord.ChannelTypeGuildText}}}},
		Time:         "10:15",
		Times:        birthdayTimeOptions(),
		Timezone:     "Europe/Oslo",
		Timezones:    []string{"UTC", "Europe/Oslo"},
		Message:      model.DefaultBirthdayMessage,
		Placeholders: birthdayMessagePlaceholders(),
	}

	var output strings.Builder
	require.NoError(t, partials.SettingsBirthday(data).Render(context.Background(), &output))
	html := output.String()
	for _, name := range []string{"enabled", "channel", "time", "timezone", "birthday_message", "birthday_message_v2", "birthday_message_v2_json"} {
		assert.Contains(t, html, `name="`+name+`"`, name)
	}
	assert.Contains(t, html, `value="10:15" selected`)
	assert.Contains(t, html, `value="23:45"`)
	assert.Len(t, data.Times, 96)
	assert.Contains(t, html, `list="birthday-timezones"`)
	assert.Contains(t, html, `value="Europe/Oslo"`)
	assert.Contains(t, html, "{{Age}}")
}

func TestHandleSaveBirthdayRejectsInvalidInputWithoutMutation(t *testing.T) {
	client := setupBirthdayWebTest(t)

	tests := []struct {
		name   string
		mutate func(url.Values)
	}{
		{name: "channel", mutate: func(v url.Values) { v.Set("channel", "not-an-id") }},
		{name: "time", mutate: func(v url.Values) { v.Set("time", "10:01") }},
		{name: "timezone", mutate: func(v url.Values) { v.Set("timezone", "UTC+02:00") }},
		{name: "mustache", mutate: func(v url.Values) { v.Set("birthday_message", "{{#broken}}") }},
		{name: "components", mutate: func(v url.Values) {
			v.Set("birthday_message_v2", "true")
			v.Set("birthday_message_v2_json", "not json")
		}},
		{name: "enable without channel", mutate: func(v url.Values) {
			v.Set("enabled", "true")
			v.Set("channel", "")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := model.GetGuildSettings(10)
			require.NoError(t, err)
			model.ResetBirthdaySettings(settings)
			require.NoError(t, model.SetGuildSettings(settings))

			values := validBirthdayForm()
			tt.mutate(values)
			response := performBirthdaySettingsRequest(t, client, values)
			body, err := io.ReadAll(response.Result().Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), `role="alert"`)

			stored, err := model.GetGuildSettings(10)
			require.NoError(t, err)
			assert.False(t, stored.BirthdayEnabled)
			assert.Zero(t, stored.BirthdayChannel)
			assert.Equal(t, model.DefaultBirthdayTimezone, stored.BirthdayTimezone)
			assert.Equal(t, model.DefaultBirthdayMessage, stored.BirthdayMessage)
		})
	}
}

func TestHandleSaveBirthdayPersistsV1AndV2WithoutClobberingOtherSettings(t *testing.T) {
	client := setupBirthdayWebTest(t)
	settings, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	settings.AntiSpamCount = 9
	require.NoError(t, model.SetGuildSettings(settings))

	v1 := validBirthdayForm()
	v1.Set("enabled", "true")
	response := performBirthdaySettingsRequest(t, client, v1)
	assert.Equal(t, http.StatusOK, response.Code)
	stored, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.True(t, stored.BirthdayEnabled)
	assert.Equal(t, "Europe/Oslo", stored.BirthdayTimezone)
	assert.Equal(t, 15, stored.BirthdayMinute)
	assert.Equal(t, 9, stored.AntiSpamCount)

	v2 := validBirthdayForm()
	v2.Set("birthday_message_v2", "true")
	v2.Set("birthday_message_v2_json", `[ {"type":10,"content":"Happy {{User.Mention}}"} ]`)
	response = performBirthdaySettingsRequest(t, client, v2)
	assert.Equal(t, http.StatusOK, response.Code)
	stored, err = model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.True(t, stored.BirthdayMessageV2)
	assert.Equal(t, `[{"type":10,"content":"Happy {{User.Mention}}"}]`, stored.BirthdayMessageV2Json)
	assert.Equal(t, 9, stored.AntiSpamCount)
}

func TestHandleSaveBirthdayTrimsTimezone(t *testing.T) {
	client := setupBirthdayWebTest(t)
	values := validBirthdayForm()
	values.Set("timezone", " Europe/Oslo ")

	response := performBirthdaySettingsRequest(t, client, values)
	assert.Equal(t, http.StatusOK, response.Code)

	stored, err := model.GetGuildSettings(10)
	require.NoError(t, err)
	assert.Equal(t, "Europe/Oslo", stored.BirthdayTimezone)
}

func validBirthdayForm() url.Values {
	return url.Values{
		"enabled":                  {"false"},
		"channel":                  {"100"},
		"time":                     {"10:15"},
		"timezone":                 {"Europe/Oslo"},
		"birthday_message":         {model.DefaultBirthdayMessage},
		"birthday_message_v2":      {"false"},
		"birthday_message_v2_json": {""},
	}
}

func setupBirthdayWebTest(t *testing.T) *bot.Client {
	t.Helper()
	original := model.DB
	db, err := model.InitDB(filepath.Join(t.TempDir(), "birthday-web.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = original
	})

	caches := cache.New(cache.WithCaches(cache.FlagGuilds, cache.FlagChannels))
	caches.AddGuild(discord.Guild{ID: 10, Name: "Test Guild", OwnerID: 20})
	var channel discord.GuildTextChannel
	require.NoError(t, json.Unmarshal([]byte(`{"id":"100","guild_id":"10","type":0,"name":"birthdays","position":0}`), &channel))
	caches.AddChannel(channel)
	return &bot.Client{Caches: caches}
}

func performBirthdaySettingsRequest(t *testing.T, client *bot.Client, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/guild/10/settings/birthday", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "10")
	request = request.WithContext(setSession(request.Context(), &model.DashboardSession{UserID: 20}))
	response := httptest.NewRecorder()
	handleSaveBirthday(client).ServeHTTP(response, request)
	return response
}

func birthdayMessagePlaceholders() []utils.MessageTemplatePlaceholder {
	return append(append([]utils.MessageTemplatePlaceholder(nil), utils.MessageTemplatePlaceholders...),
		utils.MessageTemplatePlaceholder{Placeholder: "{{Age}}", Description: "Age when a birth year is available"},
		utils.MessageTemplatePlaceholder{Placeholder: "{{#HasAge}}...{{/HasAge}}", Description: "Content shown only when age is available"},
	)
}

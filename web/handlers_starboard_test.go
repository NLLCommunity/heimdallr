package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/NLLCommunity/heimdallr/model"
	starboardsvc "github.com/NLLCommunity/heimdallr/starboard"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
)

func TestParseStarboardBoardForm(t *testing.T) {
	tests := []struct {
		name    string
		values  url.Values
		want    model.Starboard
		wantErr string
	}{
		{
			name: "valid values are normalized",
			values: url.Values{
				"name": {" Highlights "}, "channel": {"200"}, "emoji": {"⭐\ufe0f"},
				"threshold": {"4"}, "enabled": {"true"},
			},
			want: model.Starboard{ID: 7, GuildID: 100, Name: "Highlights", ChannelID: 200, Emoji: "⭐", Threshold: 4, Enabled: true},
		},
		{name: "empty name", values: url.Values{"channel": {"200"}, "emoji": {"⭐"}, "threshold": {"4"}}, wantErr: "name"},
		{name: "invalid channel", values: url.Values{"name": {"Stars"}, "channel": {"elsewhere"}, "emoji": {"⭐"}, "threshold": {"4"}}, wantErr: "channel"},
		{name: "zero threshold", values: url.Values{"name": {"Stars"}, "channel": {"200"}, "emoji": {"⭐"}, "threshold": {"0"}}, wantErr: "threshold"},
		{name: "reserved negative emoji", values: url.Values{"name": {"Stars"}, "channel": {"200"}, "emoji": {"❌\ufe0f"}, "threshold": {"4"}}, wantErr: "emoji"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStarboardBoardForm(tc.values, 100, 7)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, strings.ToLower(err.Error()), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFindStarboardIsGuildScoped(t *testing.T) {
	setupStarboardWebDB(t)
	require.NoError(t, model.DB.Create(&model.Starboard{GuildID: 100, ChannelID: 200, Name: "Stars", Emoji: "⭐", Threshold: 4}).Error)
	var stored model.Starboard
	require.NoError(t, model.DB.First(&stored).Error)

	_, err := findStarboard(101, stored.ID)
	require.Error(t, err)

	got, err := findStarboard(100, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, snowflake.ID(100), got.GuildID)
}

func TestStarboardCreateRequiresGuildAdministrator(t *testing.T) {
	client := starboardWebClient(t, 20)
	request := httptest.NewRequest(http.MethodPost, "/guild/100/starboards", strings.NewReader(url.Values{
		"name": {"Stars"}, "channel": {"200"}, "emoji": {"⭐"}, "threshold": {"4"},
	}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "100")
	request = request.WithContext(setSession(request.Context(), &model.DashboardSession{UserID: 30}))
	response := httptest.NewRecorder()

	handleStarboardCreate(client).ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Contains(t, response.Body.String(), "administrator")
}

func TestStarboardCreatePersistsValidatedBoard(t *testing.T) {
	db := setupStarboardWebDB(t)
	client := starboardWebClient(t, 20)
	original := starboardsvc.Default
	transport := &starboardWebTransport{}
	starboardsvc.Default = starboardsvc.New(db, transport)
	t.Cleanup(func() { starboardsvc.Default = original })
	request := starboardRequest(http.MethodPost, "/guild/100/starboards", url.Values{
		"name": {"Highlights"}, "channel": {"200"}, "emoji": {"⭐\ufe0f"}, "threshold": {"4"}, "enabled": {"true"},
	}, 20)
	response := httptest.NewRecorder()

	handleStarboardCreate(client).ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	var boards []model.Starboard
	require.NoError(t, model.DB.Where("guild_id = ?", 100).Find(&boards).Error)
	require.Len(t, boards, 1)
	assert.Equal(t, "⭐", boards[0].Emoji)
	assert.Equal(t, snowflake.ID(200), boards[0].ChannelID)
	assert.Equal(t, 1, transport.validations)
}

func TestStarboardSaveDoesNotAcceptBoardFromAnotherGuild(t *testing.T) {
	db := setupStarboardWebDB(t)
	client := starboardWebClient(t, 20)
	original := starboardsvc.Default
	starboardsvc.Default = starboardsvc.New(db, &starboardWebTransport{})
	t.Cleanup(func() { starboardsvc.Default = original })
	foreign := model.Starboard{GuildID: 999, ChannelID: 300, Name: "Foreign", Emoji: "⭐", Threshold: 4}
	require.NoError(t, model.DB.Create(&foreign).Error)
	request := starboardRequest(http.MethodPost, "/guild/100/starboards/1", url.Values{
		"name": {"Changed"}, "channel": {"200"}, "emoji": {"⭐"}, "threshold": {"4"},
	}, 20)
	request.SetPathValue("boardID", "1")
	response := httptest.NewRecorder()

	handleStarboardSave(client).ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	var stored model.Starboard
	require.NoError(t, model.DB.First(&stored, foreign.ID).Error)
	assert.Equal(t, "Foreign", stored.Name)
}

func TestStarboardDestinationChannelsOnlyIncludesGuildText(t *testing.T) {
	groups := []components.ChannelGroup{
		{Channels: []components.ChannelInfo{
			{ID: "1", Name: "text", Type: discord.ChannelTypeGuildText},
			{ID: "2", Name: "announcements", Type: discord.ChannelTypeGuildNews},
		}},
		{Name: "Empty after filtering", Channels: []components.ChannelInfo{
			{ID: "3", Name: "voice", Type: discord.ChannelTypeGuildVoice},
		}},
	}

	assert.Equal(t, []components.ChannelGroup{{Channels: []components.ChannelInfo{
		{ID: "1", Name: "text", Type: discord.ChannelTypeGuildText},
	}}}, starboardDestinationChannels(groups))
}

func TestStarboardSaveCanDisableBoardWithMissingUnchangedDestination(t *testing.T) {
	db := setupStarboardWebDB(t)
	client := starboardWebClient(t, 20)
	transport := &starboardWebTransport{}
	original := starboardsvc.Default
	starboardsvc.Default = starboardsvc.New(db, transport)
	t.Cleanup(func() { starboardsvc.Default = original })
	board := model.Starboard{GuildID: 100, ChannelID: 999, Name: "Old board", Emoji: "⭐", Threshold: 4, Enabled: true}
	require.NoError(t, model.DB.Create(&board).Error)
	request := starboardRequest(http.MethodPost, fmt.Sprintf("/guild/100/starboards/%d", board.ID), url.Values{
		"name": {"Old board"}, "channel": {"999"}, "emoji": {"⭐"}, "threshold": {"4"}, "enabled": {"false"},
	}, 20)
	request.SetPathValue("boardID", strconv.FormatUint(board.ID, 10))
	response := httptest.NewRecorder()

	handleStarboardSave(client).ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	var stored model.Starboard
	require.NoError(t, model.DB.First(&stored, board.ID).Error)
	assert.False(t, stored.Enabled)
	assert.Zero(t, transport.validations)
}

func TestStarboardSaveRejectsChangedMissingDestinationWhileDisabled(t *testing.T) {
	db := setupStarboardWebDB(t)
	client := starboardWebClient(t, 20)
	transport := &starboardWebTransport{}
	original := starboardsvc.Default
	starboardsvc.Default = starboardsvc.New(db, transport)
	t.Cleanup(func() { starboardsvc.Default = original })
	board := model.Starboard{GuildID: 100, ChannelID: 999, Name: "Old board", Emoji: "⭐", Threshold: 4, Enabled: true}
	require.NoError(t, model.DB.Create(&board).Error)
	request := starboardRequest(http.MethodPost, fmt.Sprintf("/guild/100/starboards/%d", board.ID), url.Values{
		"name": {"Old board"}, "channel": {"998"}, "emoji": {"⭐"}, "threshold": {"4"}, "enabled": {"false"},
	}, 20)
	request.SetPathValue("boardID", strconv.FormatUint(board.ID, 10))
	response := httptest.NewRecorder()

	handleStarboardSave(client).ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	var stored model.Starboard
	require.NoError(t, model.DB.First(&stored, board.ID).Error)
	assert.Equal(t, snowflake.ID(999), stored.ChannelID)
	assert.Zero(t, transport.validations)
}

func setupStarboardWebDB(t *testing.T) *gorm.DB {
	t.Helper()
	original := model.DB
	db, err := model.InitDB(filepath.Join(t.TempDir(), "starboard-web.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = original
	})
	return db
}

func starboardWebClient(t *testing.T, ownerID snowflake.ID) *bot.Client {
	t.Helper()
	caches := cache.New(cache.WithCaches(cache.FlagGuilds, cache.FlagMembers, cache.FlagRoles, cache.FlagChannels))
	caches.AddGuild(discord.Guild{ID: 100, Name: "Test Guild", OwnerID: ownerID})
	caches.AddRole(discord.Role{ID: 100, GuildID: 100, Name: "@everyone"})
	caches.AddMember(discord.Member{GuildID: 100, User: discord.User{ID: 30, Username: "member"}})
	var channel discord.GuildTextChannel
	require.NoError(t, json.Unmarshal([]byte(`{"id":"200","guild_id":"100","type":0,"name":"highlights","position":0}`), &channel))
	caches.AddChannel(channel)
	return &bot.Client{Caches: caches}
}

func starboardRequest(method, target string, values url.Values, userID snowflake.ID) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "100")
	return request.WithContext(setSession(request.Context(), &model.DashboardSession{UserID: userID}))
}

type starboardWebTransport struct{ validations int }

func (t *starboardWebTransport) ValidateBoard(model.Starboard) error { t.validations++; return nil }
func (*starboardWebTransport) Source(snowflake.ID, snowflake.ID, snowflake.ID) (*discord.Message, error) {
	return nil, errors.New("unused")
}
func (*starboardWebTransport) Eligible(model.Starboard, *discord.Message) (bool, snowflake.ID, error) {
	return false, 0, errors.New("unused")
}
func (*starboardWebTransport) Votes(snowflake.ID, snowflake.ID, string) (starboardsvc.Votes, error) {
	return starboardsvc.Votes{}, errors.New("unused")
}
func (*starboardWebTransport) Publish(model.Starboard, *discord.Message, starboardsvc.Votes, string) (snowflake.ID, error) {
	return 0, errors.New("unused")
}
func (*starboardWebTransport) Recover(model.Starboard, string, time.Time) (snowflake.ID, error) {
	return 0, errors.New("unused")
}
func (*starboardWebTransport) Update(model.Starboard, *discord.Message, snowflake.ID, starboardsvc.Votes) error {
	return errors.New("unused")
}
func (*starboardWebTransport) Delete(snowflake.ID, snowflake.ID) error { return errors.New("unused") }

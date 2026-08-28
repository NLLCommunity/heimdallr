package listeners

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/NLLCommunity/heimdallr/model"
)

func TestBirthdayMemberLeaveDeletesOnlyDepartedGuildRecord(t *testing.T) {
	original := model.DB
	db, err := model.InitDB(filepath.Join(t.TempDir(), "birthday-leave.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = original
	})

	_, err = model.SetBirthday(10, 20, 26, time.August, nil, "", 2026)
	require.NoError(t, err)
	_, err = model.SetBirthday(11, 20, 27, time.August, nil, "", 2026)
	require.NoError(t, err)

	OnBirthdayMemberLeave(&events.GuildMemberLeave{
		GuildID: 10,
		User:    discord.User{ID: 20},
	})

	_, err = model.GetBirthday(10, 20)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	otherGuild, err := model.GetBirthday(11, 20)
	require.NoError(t, err)
	assert.Equal(t, 27, otherGuild.Day)
}

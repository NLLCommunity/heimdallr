package listeners

import (
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/starboard"
	"github.com/disgoorg/disgo/events"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestStarboardListenersPersistClearUpdateAndDeletion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "events.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.GuildSettings{}, &model.StarboardEntry{}, &model.StarboardJob{}))
	require.NoError(t, db.Create(&model.GuildSettings{GuildID: 10, StarboardEnabled: true}).Error)
	require.NoError(t, db.Create(&model.StarboardEntry{GuildID: 10, BoardID: 1, ChannelID: 30, MessageID: 100}).Error)
	old := starboard.Default
	starboard.Default = starboard.New(db, nil)
	t.Cleanup(func() { starboard.Default = old; sql, _ := db.DB(); sql.Close() })
	OnStarboardReactionRemoveAll(&events.GuildMessageReactionRemoveAll{GuildID: 10, ChannelID: 30, MessageID: 100})
	OnStarboardMessageUpdate(&events.GuildMessageUpdate{GenericGuildMessage: &events.GenericGuildMessage{GuildID: 10, ChannelID: 30, MessageID: 100}})
	OnStarboardMessageDelete(&events.GuildMessageDelete{GenericGuildMessage: &events.GenericGuildMessage{GuildID: 10, ChannelID: 30, MessageID: 100}})
	var jobs []model.StarboardJob
	require.NoError(t, db.Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.True(t, jobs[0].Deleted)
	require.Equal(t, uint64(100), uint64(jobs[0].MessageID))
}

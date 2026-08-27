package listeners

import (
	"log/slog"

	"github.com/disgoorg/disgo/events"

	"github.com/NLLCommunity/heimdallr/model"
)

func OnBirthdayMemberLeave(event *events.GuildMemberLeave) {
	if err := model.DeleteBirthday(event.GuildID, event.User.ID); err != nil {
		slog.Error(
			"failed to delete birthday for departing member",
			"guild_id", event.GuildID,
			"user_id", event.User.ID,
			"error", err,
		)
	}
}

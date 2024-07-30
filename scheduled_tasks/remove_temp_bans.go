package scheduled_tasks

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/rest"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/task"
)

func RemoveTempBansScheduledTask(client bot.Client) task.Task {
	values := map[string]any{
		"client": client,
	}

	t := task.New("remove-temp-bans", removeTempBans, values, 15*time.Minute)
	t.StartNoWait()

	return t
}

func removeTempBans(ctx context.Context) {
	client, hasClient := ctx.Value("client").(bot.Client)
	if !hasClient {
		return
	}

	tb, err := model.GetExpiredTempBans()
	if err != nil {
		return
	}

	for _, ban := range tb {
		err = client.Rest().DeleteBan(ban.GuildID, ban.UserID, rest.WithReason("Ban expired."))
		if err != nil {
			slog.Error("Failed to delete temp ban from Discord.",
				"guild_id", ban.GuildID,
				"user_id", ban.UserID,
				"error", err)
		}

		err = ban.Delete()
		if err != nil {
			slog.Error("Failed to remove temp ban from database.")
		}
	}
}

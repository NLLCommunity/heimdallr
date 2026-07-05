package scheduled_tasks

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/task"
)

func RemoveTempBansScheduledTask(client *bot.Client) task.Task {
	values := task.ContextKeyMap{
		task.ContextKeyBotClientRef: client,
	}

	t := task.New("remove-temp-bans", removeTempBans, values, 15*time.Minute, true)
	t.StartNoWait()

	return t
}

func removeTempBans(ctx context.Context) {
	client, hasClient := ctx.Value(task.ContextKeyBotClientRef).(*bot.Client)
	if !hasClient {
		slog.Error("could not retrieve client for removing temp bans")
		return
	}

	tb, err := model.GetExpiredTempBans()
	if err != nil {
		slog.Error("Failed to get expired temp bans.", "error", err)
		return
	}

	for _, ban := range tb {
		err := removeTempBan(ban, client.Rest)
		if err != nil {
			slog.Error(
				"Failed to remove temp ban.",
				"guild_id", ban.GuildID,
				"user_id", ban.UserID,
				"error", err,
			)
		}
	}
}

func removeTempBan(ban model.TempBan, r rest.Rest) error {
	err := r.DeleteBan(ban.GuildID, ban.UserID, rest.WithReason("Ban expired."))
	// A missing ban (e.g. a moderator already unbanned the user) means there is
	// nothing left to undo, so treat it the same as a successful unban and drop
	// the row. Any other error is left to be retried on the next run.
	if err == nil || rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownBan) {
		return ban.Delete()
	}

	guildSettings, err := model.GetGuildSettings(ban.GuildID)
	if err != nil {
		return err
	}

	if guildSettings.ModeratorChannel == 0 {
		return nil
	}

	user, err := r.GetUser(ban.UserID)
	if err != nil {
		return err
	}
	_, err = r.CreateMessage(guildSettings.ModeratorChannel,
		discord.NewMessageCreate().
			WithContentf("Failed to unban temporarily banned user %s (`%d`)", user.Username, user.ID))
	return err
}

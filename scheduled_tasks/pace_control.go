package scheduled_tasks

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/listeners"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/task"
	"github.com/NLLCommunity/heimdallr/utils"
)

// lastSetSlowmode tracks the last slow mode value set per channel to avoid unnecessary API calls.
var lastSetSlowmode sync.Map

func PaceControlTask(client *bot.Client) task.Task {
	values := task.ContextKeyMap{
		task.ContextKeyBotClientRef: client,
	}

	t := task.New("pace-control", paceControlEvaluate, values, 30*time.Second)
	t.Start()

	return t
}

func paceControlEvaluate(ctx context.Context) {
	client, ok := ctx.Value(task.ContextKeyBotClientRef).(*bot.Client)
	if !ok {
		slog.Warn("pace-control: failed to get client from context (type assertion failed)")
		return
	}

	keys := listeners.ActiveChannelKeys()
	slog.Debug("pace-control: evaluating", "active_channels", len(keys))

	for _, key := range keys {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			slog.Warn("pace-control: invalid key format", "key", key)
			continue
		}

		guildID, err := snowflake.Parse(parts[0])
		if err != nil {
			slog.Warn("pace-control: invalid guild ID in key", "key", key, "error", err)
			continue
		}
		channelID, err := snowflake.Parse(parts[1])
		if err != nil {
			slog.Warn("pace-control: invalid channel ID in key", "key", key, "error", err)
			continue
		}

		pc, err := model.GetPaceControl(guildID, channelID)
		if err != nil {
			slog.Debug("pace-control: channel not configured, skipping",
				"guild", guildID, "channel", channelID, "error", err)
			continue
		}
		if !pc.Enabled {
			slog.Debug("pace-control: channel disabled, skipping",
				"guild", guildID, "channel", channelID)
			continue
		}

		wpmWindow := time.Duration(pc.WPMWindowSeconds) * time.Second
		userWindow := time.Duration(pc.UserWindowSeconds) * time.Second
		if wpmWindow <= 0 {
			wpmWindow = 60 * time.Second
		}
		if userWindow <= 0 {
			userWindow = 120 * time.Second
		}

		totalWords, activeUsers := listeners.GetPaceStats(key, wpmWindow, userWindow)
		// Scale words in the window to words-per-minute
		measuredWPM := int(float64(totalWords) * 60 / wpmWindow.Seconds())

		currentSlowmode := 0
		if prev, loaded := lastSetSlowmode.Load(key); loaded {
			currentSlowmode = prev.(int)
		}

		// If below the activation threshold, ease back to minSlowmode
		activationWPM := pc.ActivationWPM
		if activationWPM > 0 && measuredWPM < activationWPM {
			slog.Debug("pace-control: below activation threshold",
				"channel", channelID, "wpm", measuredWPM, "activation", activationWPM)
		}

		slowmode := calculateSlowmode(measuredWPM, activeUsers, pc.TargetWPM, pc.MinSlowmode, pc.MaxSlowmode, currentSlowmode, activationWPM)

		slog.Debug("pace-control: channel stats",
			"guild", guildID,
			"channel", channelID,
			"wpm", measuredWPM,
			"active_users", activeUsers,
			"target_wpm", pc.TargetWPM,
			"current_slowmode", currentSlowmode,
			"new_slowmode", slowmode,
			"min", pc.MinSlowmode,
			"max", pc.MaxSlowmode,
		)

		// Only update if the value changed
		if prev, loaded := lastSetSlowmode.Load(key); loaded && prev.(int) == slowmode {
			slog.Debug("pace-control: slowmode unchanged, skipping API call",
				"channel", channelID, "slowmode", slowmode)
			continue
		}

		slog.Debug("pace-control: setting slowmode",
			"guild", guildID,
			"channel", channelID,
			"slowmode", slowmode,
		)

		_, err = client.Rest.UpdateChannel(channelID, discord.GuildTextChannelUpdate{
			RateLimitPerUser: utils.Ref(slowmode),
		})
		if err != nil {
			slog.Error("pace-control: failed to update channel slow mode",
				"guild", guildID,
				"channel", channelID,
				"error", err,
			)
			continue
		}

		slog.Debug("pace-control: slowmode updated successfully",
			"guild", guildID, "channel", channelID, "slowmode", slowmode)
		lastSetSlowmode.Store(key, slowmode)
	}
}

func calculateSlowmode(measuredWPM, activeUsers, targetWPM, minSlowmode, maxSlowmode, currentSlowmode, activationWPM int) int {
	if targetWPM <= 0 {
		return currentSlowmode
	}

	if activeUsers <= 0 {
		activeUsers = 1
	}

	// Calculate the ideal slowmode to bring WPM toward the target.
	var idealSlowmode int
	if activationWPM > 0 && measuredWPM < activationWPM {
		// Below activation threshold — stay at min (dormant).
		idealSlowmode = minSlowmode
	} else if measuredWPM <= targetWPM {
		// Above activation but at or below target — ease toward min.
		idealSlowmode = minSlowmode
	} else {
		// Above target — calculate what per-user delay would bring WPM down.
		avgWordsPerMsg := float64(measuredWPM) / float64(activeUsers)
		idealSlowmode = int((float64(activeUsers) * 60) / (float64(targetWPM) / avgWordsPerMsg))
		idealSlowmode = min(max(idealSlowmode, minSlowmode), maxSlowmode)
	}

	// Move halfway toward the ideal each tick (exponential ease).
	// This adapts quickly to large differences while still smoothing small ones.
	// +1/-1 ensures we always make progress and don't get stuck.
	diff := idealSlowmode - currentSlowmode
	step := diff / 2
	if diff > 0 && step == 0 {
		step = 1
	} else if diff < 0 && step == 0 {
		step = -1
	}
	slowmode := currentSlowmode + step

	// Final clamp to absolute bounds.
	return min(max(slowmode, minSlowmode), maxSlowmode)
}

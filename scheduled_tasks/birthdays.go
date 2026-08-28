package scheduled_tasks

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/task"
	"github.com/NLLCommunity/heimdallr/utils"
)

type birthdaySender interface {
	Send(context.Context, *model.GuildSettings, model.Birthday, int) error
}

func BirthdayAnnouncementsScheduledTask(client *bot.Client) task.Task {
	exec := func(ctx context.Context) {
		runBirthdayAnnouncements(ctx, time.Now().UTC(), &discordBirthdaySender{client: client})
	}
	scheduled := task.NewScheduled(
		"birthday-announcements",
		exec,
		nil,
		task.MustCron("*/15 * * * *"),
		true,
	)
	scheduled.StartNoWait()
	return scheduled
}

func runBirthdayAnnouncements(ctx context.Context, now time.Time, sender birthdaySender) {
	settingsRows, err := model.GetBirthdayAnnouncementGuilds()
	if err != nil {
		slog.Error("failed to load birthday announcement guilds", "error", err)
		return
	}

	for i := range settingsRows {
		if ctx.Err() != nil {
			return
		}
		settings := &settingsRows[i]
		location, err := utils.ValidateTimezone(settings.BirthdayTimezone)
		if err != nil {
			slog.Error("invalid birthday timezone", "guild_id", settings.GuildID, "timezone", settings.BirthdayTimezone, "error", err)
			continue
		}
		localNow := now.In(location)
		currentMinute := localNow.Hour()*60 + localNow.Minute()
		configuredMinute := settings.BirthdayHour*60 + settings.BirthdayMinute
		if currentMinute < configuredMinute {
			continue
		}

		birthdays, err := model.DueBirthdays(settings.GuildID, localNow)
		if err != nil {
			slog.Error("failed to load due birthdays", "guild_id", settings.GuildID, "error", err)
			continue
		}
		for _, birthday := range birthdays {
			if ctx.Err() != nil {
				return
			}
			if err := sender.Send(ctx, settings, birthday, localNow.Year()); err != nil {
				slog.Error("failed to send birthday announcement", "guild_id", settings.GuildID, "user_id", birthday.UserID, "error", err)
				continue
			}
			if err := model.MarkBirthdayAnnounced(settings.GuildID, birthday.UserID, localNow.Year()); err != nil {
				slog.Error("failed to mark birthday announced", "guild_id", settings.GuildID, "user_id", birthday.UserID, "error", err)
			}
		}
	}
}

type discordBirthdaySender struct {
	client *bot.Client
}

func (s *discordBirthdaySender) Send(
	_ context.Context,
	settings *model.GuildSettings,
	birthday model.Birthday,
	localYear int,
) error {
	if s.client == nil {
		return fmt.Errorf("birthday sender has no Discord client")
	}

	guild, ok := s.client.Caches.Guild(settings.GuildID)
	if !ok {
		restGuild, err := s.client.Rest.GetGuild(settings.GuildID, false)
		if err != nil {
			return fmt.Errorf("get guild: %w", err)
		}
		guild = restGuild.Guild
	}
	member, ok := s.client.Caches.Member(settings.GuildID, birthday.UserID)
	if !ok {
		restMember, err := s.client.Rest.GetMember(settings.GuildID, birthday.UserID)
		if err != nil {
			return fmt.Errorf("get birthday member: %w", err)
		}
		member = *restMember
	}

	data := utils.NewMessageTemplateData(member, guild)
	data.Age, data.HasAge = birthdayAge(birthday.BirthYear, localYear)
	message, err := buildBirthdayMessage(
		settings,
		data,
		utils.BuildEmojiMap(s.client, settings.GuildID),
		birthday.UserID,
	)
	if err != nil {
		return err
	}
	if _, err := s.client.Rest.CreateMessage(settings.BirthdayChannel, message); err != nil {
		return fmt.Errorf("create birthday message: %w", err)
	}
	return nil
}

func birthdayAge(birthYear *int, localYear int) (string, bool) {
	if birthYear == nil || *birthYear > localYear {
		return "", false
	}
	return strconv.Itoa(localYear - *birthYear), true
}

func buildBirthdayMessage(
	settings *model.GuildSettings,
	data utils.MessageTemplateData,
	emojiMap map[string]discord.Emoji,
	userID snowflake.ID,
) (discord.MessageCreate, error) {
	allowedMentions := &discord.AllowedMentions{Users: []snowflake.ID{userID}}
	if settings.BirthdayMessageV2 {
		components, err := utils.BuildV2Message(settings.BirthdayMessageV2Json, data, emojiMap)
		if err != nil {
			return discord.MessageCreate{}, fmt.Errorf("build birthday Components V2 message: %w", err)
		}
		if len(components) == 0 {
			return discord.MessageCreate{}, fmt.Errorf("birthday Components V2 message cannot be empty")
		}
		return discord.NewMessageCreateV2(components...).WithAllowedMentions(allowedMentions), nil
	}

	content, err := mustache.RenderRaw(settings.BirthdayMessage, true, data)
	if err != nil {
		return discord.MessageCreate{}, fmt.Errorf("render birthday message: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return discord.MessageCreate{}, fmt.Errorf("birthday message cannot be empty")
	}
	return discord.NewMessageCreate().
		WithContent(content).
		WithAllowedMentions(allowedMentions), nil
}

package admin

import (
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

func BirthdayTimeAutocomplete(ctx rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
	query := strings.ToLower(strings.TrimSpace(ctx.Value))
	choices := make([]rave.Choice[string], 0, 25)
	for hour := range 24 {
		for _, minute := range []int{0, 15, 30, 45} {
			value := model.FormatBirthdayTime(hour, minute)
			if query != "" && !strings.Contains(strings.ToLower(value), query) {
				continue
			}
			choices = append(choices, rave.Choice[string]{Name: value, Value: value})
			if len(choices) == 25 {
				return choices, nil
			}
		}
	}
	return choices, nil
}

func BirthdayTimezoneAutocomplete(ctx rave.AutocompleteContext[string]) ([]rave.Choice[string], error) {
	names := utils.FilterTimezones(ctx.Value, 25)
	choices := make([]rave.Choice[string], len(names))
	for i, name := range names {
		choices[i] = rave.Choice[string]{Name: name, Value: name}
	}
	return choices, nil
}

func AdminBirthdayHandler(event *handler.CommandEvent) error {
	utils.LogInteraction("admin", event)
	guild, ok := event.Guild()
	if !ok {
		return interactions.ErrEventNoGuildID
	}
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}
	candidate := *settings
	data := event.SlashCommandInteractionData()
	changed := false
	var messages []string

	if reset, present := data.OptString("reset"); present {
		changed = true
		switch reset {
		case "enabled":
			candidate.BirthdayEnabled = false
		case "channel":
			candidate.BirthdayChannel = 0
		case "time":
			candidate.BirthdayHour = model.DefaultBirthdayHour
			candidate.BirthdayMinute = model.DefaultBirthdayMinute
		case "timezone":
			candidate.BirthdayTimezone = model.DefaultBirthdayTimezone
		case "all":
			model.ResetBirthdaySettings(&candidate)
		default:
			return event.CreateMessage(interactions.EphemeralMessageContent("Unknown birthday reset option."))
		}
		messages = append(messages, "Birthday settings reset: "+reset+".")
	}
	if enabled, present := data.OptBool("enabled"); present {
		changed = true
		candidate.BirthdayEnabled = enabled
		messages = append(messages, fmt.Sprintf("Birthday announcements enabled: %s.", utils.Iif(enabled, "yes", "no")))
	}
	if channel, present := data.OptChannel("channel"); present {
		changed = true
		candidate.BirthdayChannel = channel.ID
		messages = append(messages, fmt.Sprintf("Birthday channel set to <#%s>.", channel.ID))
	}
	if rawTime, present := data.OptString("time"); present {
		hour, minute, parseErr := model.ParseBirthdayTime(rawTime)
		if parseErr != nil {
			return event.CreateMessage(interactions.EphemeralMessageContent(parseErr.Error()))
		}
		changed = true
		candidate.BirthdayHour = hour
		candidate.BirthdayMinute = minute
		messages = append(messages, "Birthday announcement time set to "+rawTime+".")
	}
	if timezone, present := data.OptString("timezone"); present {
		timezone = strings.TrimSpace(timezone)
		changed = true
		candidate.BirthdayTimezone = timezone
		messages = append(messages, "Birthday timezone set to "+timezone+".")
	}

	if !changed {
		return event.CreateMessage(interactions.EphemeralMessageContent(birthdayInfo(settings)))
	}
	if err := model.ValidateBirthdaySettings(&candidate); err != nil {
		return event.CreateMessage(interactions.EphemeralMessageContent(err.Error()))
	}
	if err := model.UpdateGuildSettingsColumns(&candidate,
		"BirthdayEnabled",
		"BirthdayChannel",
		"BirthdayTimezone",
		"BirthdayHour",
		"BirthdayMinute",
		"BirthdayMessage",
		"BirthdayMessageV2",
		"BirthdayMessageV2Json",
	); err != nil {
		return err
	}
	logSettingsCommandUpdate(guild.ID, event.User(), "birthday", map[string]any{
		"enabled":  candidate.BirthdayEnabled,
		"channel":  candidate.BirthdayChannel.String(),
		"time":     model.FormatBirthdayTime(candidate.BirthdayHour, candidate.BirthdayMinute),
		"timezone": candidate.BirthdayTimezone,
		"mode":     utils.Iif(candidate.BirthdayMessageV2, "components_v2", "plain"),
	})
	return event.CreateMessage(interactions.EphemeralMessageContent(strings.Join(messages, "\n")))
}

func birthdayInfo(settings *model.GuildSettings) string {
	return fmt.Sprintf(
		"## Birthday settings\n**Enabled:** %s\n**Channel:** %s\n**Time:** %s\n**Timezone:** %s\n**Message mode:** %s",
		utils.Iif(settings.BirthdayEnabled, "yes", "no"),
		utils.MentionChannelOrDefault(&settings.BirthdayChannel, "not set"),
		model.FormatBirthdayTime(settings.BirthdayHour, settings.BirthdayMinute),
		settings.BirthdayTimezone,
		utils.Iif(settings.BirthdayMessageV2, "Components V2", "plain text"),
	)
}

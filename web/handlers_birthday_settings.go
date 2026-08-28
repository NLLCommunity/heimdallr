package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

func birthdayTimeOptions() []string {
	times := make([]string, 0, 96)
	for hour := range 24 {
		for _, minute := range []int{0, 15, 30, 45} {
			times = append(times, model.FormatBirthdayTime(hour, minute))
		}
	}
	return times
}

func birthdayTemplatePlaceholders() []utils.MessageTemplatePlaceholder {
	placeholders := append([]utils.MessageTemplatePlaceholder(nil), utils.MessageTemplatePlaceholders...)
	return append(placeholders,
		utils.MessageTemplatePlaceholder{Placeholder: "{{Age}}", Description: "Age when the member supplied a birth year"},
		utils.MessageTemplatePlaceholder{Placeholder: "{{#HasAge}}...{{/HasAge}}", Description: "Content shown only when an age is available"},
	)
}

func birthdaySettingsData(
	guildID string,
	settings *model.GuildSettings,
	channels []components.ChannelGroup,
) partials.BirthdayData {
	return partials.BirthdayData{
		GuildID:       guildID,
		Enabled:       settings.BirthdayEnabled,
		Channel:       idStr(settings.BirthdayChannel),
		Channels:      channels,
		Time:          model.FormatBirthdayTime(settings.BirthdayHour, settings.BirthdayMinute),
		Times:         birthdayTimeOptions(),
		Timezone:      settings.BirthdayTimezone,
		Timezones:     utils.TimezoneNames(),
		Message:       settings.BirthdayMessage,
		MessageV2:     settings.BirthdayMessageV2,
		MessageV2Json: settings.BirthdayMessageV2Json,
		Placeholders:  birthdayTemplatePlaceholders(),
	}
}

func handleSaveBirthday(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		stored, err := model.GetGuildSettings(guildID)
		if err != nil {
			renderSafe(w, r, partials.SettingsBirthday(partials.BirthdayData{
				GuildID:   guildIDStr,
				SaveError: "Failed to load settings.",
			}))
			return
		}
		settings := *stored
		settings.BirthdayEnabled = r.FormValue("enabled") == "true"
		settings.BirthdayTimezone = strings.TrimSpace(r.FormValue("timezone"))
		settings.BirthdayMessage = r.FormValue("birthday_message")
		settings.BirthdayMessageV2 = r.FormValue("birthday_message_v2") == "true"
		v2Raw := r.FormValue("birthday_message_v2_json")

		renderError := func(message string) {
			data := birthdaySettingsData(guildIDStr, &settings, guildChannels(client, guildID))
			data.MessageV2Json = v2Raw
			data.SaveError = message
			renderSafe(w, r, partials.SettingsBirthday(data))
		}

		channel, err := parseSnowflakeOrZero(r.FormValue("channel"))
		if err != nil {
			renderError("Invalid channel ID.")
			return
		}
		if channel != 0 {
			cachedChannel, exists := client.Caches.Channel(channel)
			if !exists || cachedChannel.GuildID() != guildID || !components.IsTextChannel(cachedChannel.Type()) {
				renderError("Choose a text channel from this server.")
				return
			}
		}
		settings.BirthdayChannel = channel

		hour, minute, err := model.ParseBirthdayTime(r.FormValue("time"))
		if err != nil {
			renderError(err.Error())
			return
		}
		settings.BirthdayHour = hour
		settings.BirthdayMinute = minute

		if settings.BirthdayMessageV2 {
			compact, err := validateAndCompactV2JSON(v2Raw)
			if err != nil {
				renderError("Birthday message: " + err.Error() + ".")
				return
			}
			settings.BirthdayMessageV2Json = compact
		} else {
			if _, err := mustache.RenderRaw(settings.BirthdayMessage, true, utils.MessageTemplateData{}); err != nil {
				renderError("Birthday message contains invalid placeholders.")
				return
			}
			settings.BirthdayMessageV2Json = preserveV2Json(v2Raw)
		}

		if err := model.ValidateBirthdaySettings(&settings); err != nil {
			renderError(err.Error())
			return
		}
		if err := model.UpdateGuildSettingsColumns(&settings,
			"BirthdayEnabled", "BirthdayChannel", "BirthdayTimezone",
			"BirthdayHour", "BirthdayMinute", "BirthdayMessage",
			"BirthdayMessageV2", "BirthdayMessageV2Json",
		); err != nil {
			slog.Error("failed to save birthday settings", "guild_id", guildID, "error", err)
			renderError("Failed to save settings.")
			return
		}
		logSettingsUpdate(sessionFromContext(r.Context()), guildID, "birthday", map[string]any{
			"enabled":  settings.BirthdayEnabled,
			"channel":  idStr(settings.BirthdayChannel),
			"time":     model.FormatBirthdayTime(settings.BirthdayHour, settings.BirthdayMinute),
			"timezone": settings.BirthdayTimezone,
			"mode":     utils.Iif(settings.BirthdayMessageV2, "components_v2", "plain"),
		})

		data := birthdaySettingsData(guildIDStr, &settings, guildChannels(client, guildID))
		data.SaveSuccess = true
		renderSafe(w, r, partials.SettingsBirthday(data))
	}
}

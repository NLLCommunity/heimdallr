package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/cbroglie/mustache"
)

const (
	DefaultBirthdayTimezone = "UTC"
	DefaultBirthdayHour     = 10
	DefaultBirthdayMinute   = 0
	DefaultBirthdayMessage  = "Happy birthday, {{User.Mention}}! 🎉"
)

func ResetBirthdaySettings(settings *GuildSettings) {
	settings.BirthdayEnabled = false
	settings.BirthdayChannel = 0
	settings.BirthdayTimezone = DefaultBirthdayTimezone
	settings.BirthdayHour = DefaultBirthdayHour
	settings.BirthdayMinute = DefaultBirthdayMinute
	settings.BirthdayMessage = DefaultBirthdayMessage
	settings.BirthdayMessageV2 = false
	settings.BirthdayMessageV2Json = ""
}

func ParseBirthdayTime(raw string) (hour, minute int, err error) {
	if len(raw) != 5 || raw[2] != ':' {
		return 0, 0, fmt.Errorf("birthday time must use HH:MM")
	}
	hour, err = strconv.Atoi(raw[:2])
	if err != nil {
		return 0, 0, fmt.Errorf("birthday time must use HH:MM: %w", err)
	}
	minute, err = strconv.Atoi(raw[3:])
	if err != nil {
		return 0, 0, fmt.Errorf("birthday time must use HH:MM: %w", err)
	}
	if hour < 0 || hour > 23 || (minute != 0 && minute != 15 && minute != 30 && minute != 45) {
		return 0, 0, fmt.Errorf("birthday time must be a 15-minute increment between 00:00 and 23:45")
	}
	return hour, minute, nil
}

func FormatBirthdayTime(hour, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func ValidateBirthdaySettings(settings *GuildSettings) error {
	if _, _, err := ParseBirthdayTime(FormatBirthdayTime(settings.BirthdayHour, settings.BirthdayMinute)); err != nil {
		return err
	}
	if _, err := utils.ValidateTimezone(settings.BirthdayTimezone); err != nil {
		return err
	}
	if settings.BirthdayEnabled && settings.BirthdayChannel == 0 {
		return fmt.Errorf("a birthday announcement channel is required when birthdays are enabled")
	}

	if settings.BirthdayMessageV2 {
		if strings.TrimSpace(settings.BirthdayMessageV2Json) == "" {
			return fmt.Errorf("birthday Components V2 JSON cannot be empty")
		}
		return nil
	}
	if strings.TrimSpace(settings.BirthdayMessage) == "" {
		return fmt.Errorf("birthday message cannot be empty")
	}
	if _, err := mustache.RenderRaw(settings.BirthdayMessage, true, utils.MessageTemplateData{}); err != nil {
		return fmt.Errorf("invalid birthday message template: %w", err)
	}
	return nil
}

func GetBirthdayAnnouncementGuilds() ([]GuildSettings, error) {
	var settings []GuildSettings
	err := DB.Where("birthday_enabled = ? AND birthday_channel <> 0", true).Find(&settings).Error
	return settings, err
}

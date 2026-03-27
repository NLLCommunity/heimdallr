package admin

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var antiSpamSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "anti-spam",
	Description: "View or set anti-spam settings",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionBool{
			Name:        "enabled",
			Description: "Whether to enable the anti-spam system",
			Required:    false,
		},
		discord.ApplicationCommandOptionInt{
			Name:        "count",
			Description: "The number of messages allowed before Heimdallr takes action (within the cooldown period)",
			Required:    false,
			MinValue:    utils.Ref(1),
			MaxValue:    utils.Ref(15),
		},
		discord.ApplicationCommandOptionInt{
			Name:        "cooldown",
			Description: "The time in seconds to wait before resetting the message count",
			Required:    false,
			MinValue:    utils.Ref(1),
			MaxValue:    utils.Ref(60),
		},
		discord.ApplicationCommandOptionInt{
			Name:        "timeout",
			Description: "The time in minutes to timeout a user who has exceeded the message count",
			Required:    false,
			MinValue:    utils.Ref(1),
			MaxValue:    utils.Ref(10080), // 7 days
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset a setting to its default value",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Enabled", Value: "enabled"},
				{Name: "Count", Value: "count"},
				{Name: "Cooldown", Value: "cooldown"},
				{Name: "All", Value: "all"},
			},
		},
	},
}

func antiSpamInfo(settings *model.GuildSettings) string {
	antispamEnabledInfo := "> This determines whether to enable the anti-spam system."
	antispamEnabled := fmt.Sprintf(
		"**Anti-spam enabled:** %s\n%s",
		utils.Iif(settings.AntiSpamEnabled, "yes", "no"), antispamEnabledInfo,
	)

	antispamCountInfo := "> This is the number of messages allowed before Heimdallr takes action (within the cooldown period)."
	antispamCount := fmt.Sprintf(
		"**Anti-spam count:** %d\n%s",
		settings.AntiSpamCount, antispamCountInfo,
	)

	antispamCooldownInfo := "> This is the time in seconds to wait before resetting the message count."
	antispamCooldown := fmt.Sprintf(
		"**Anti-spam cooldown:** %d\n%s",
		settings.AntiSpamCooldownSeconds, antispamCooldownInfo,
	)

	antispamTimeoutInfo := "> This is the time in minutes to timeout a user who has exceeded the message count."
	antispamTimeout := fmt.Sprintf(
		"**Anti-spam timeout:** %d\n%s",
		settings.AntiSpamTimeoutMinutes, antispamTimeoutInfo,
	)

	return fmt.Sprintf(
		"## Anti-spam settings\n%s\n\n%s\n\n%s\n\n%s",
		antispamEnabled, antispamCount, antispamCooldown, antispamTimeout,
	)
}

func AdminAntiSpamHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		slog.Warn("Failed to get guild settings", "err", err)
		return err
	}

	message := ""

	resetOption, hasReset := data.OptString("reset")
	if hasReset {
		switch resetOption {
		case "enabled":
			settings.AntiSpamEnabled = false
			message += "Anti-spam enabled has been reset.\n"
		case "count":
			settings.AntiSpamCount = 5 // Default value
			message += "Anti-spam count has been reset.\n"
		case "cooldown":
			settings.AntiSpamCooldownSeconds = 20 // Default value
			message += "Anti-spam cooldown has been reset.\n"
		case "timeout":
			settings.AntiSpamTimeoutMinutes = 720 // Default value: 12 hours
			message += "Anti-spam timeout has been reset.\n"
		case "all":
			settings.AntiSpamEnabled = false
			settings.AntiSpamCount = 5            // Default value
			settings.AntiSpamCooldownSeconds = 20 // Default value
			message += "All anti-spam settings have been reset.\n"
		}
	}

	enabled, hasEnabled := data.OptBool("enabled")
	if hasEnabled {
		settings.AntiSpamEnabled = enabled
		message += fmt.Sprintf("Anti-spam enabled set to %s\n", utils.Iif(enabled, "yes", "no"))
	}

	count, hasCount := data.OptInt("count")
	if hasCount {
		settings.AntiSpamCount = count
		message += fmt.Sprintf("Anti-spam count (message threshold within cooldown period) set to %d\n", count)
	}

	cooldown, hasCooldown := data.OptInt("cooldown")
	if hasCooldown {
		settings.AntiSpamCooldownSeconds = cooldown
		message += fmt.Sprintf("Anti-spam cooldown (seconds) set to %d\n", cooldown)
	}

	timeout, hasTimeout := data.OptInt("timeout")
	if hasTimeout {
		settings.AntiSpamTimeoutMinutes = timeout
		message += fmt.Sprintf("Anti-spam timeout (minutes) set to %d\n", timeout)
	}

	if !utils.Any(hasEnabled, hasCount, hasCooldown, hasReset) {
		return e.CreateMessage(interactions.EphemeralMessageContent(antiSpamInfo(settings)))
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		slog.Warn("Failed to set guild settings", "err", err)
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

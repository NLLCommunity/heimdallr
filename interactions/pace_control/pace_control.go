package pace_control

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Route("/pace-control", func(r handler.Router) {
		r.Command("/enable", enableHandler)
		r.Command("/disable", disableHandler)
		r.Command("/status", statusHandler)
		r.Command("/override", overrideHandler)
	})

	r.Command("/slow-mode", slowModeAliasHandler)

	return []discord.ApplicationCommandCreate{paceControlCommand, slowModeCommand}
}

var channelOption = discord.ApplicationCommandOptionChannel{
	Name:        "channel",
	Description: "Target channel (defaults to current channel)",
	Required:    false,
	ChannelTypes: []discord.ChannelType{
		discord.ChannelTypeGuildText,
	},
}

var paceControlCommand = discord.SlashCommandCreate{
	Name:                     "pace-control",
	Description:              "Manage automatic slow mode based on channel activity",
	DefaultMemberPermissions: omit.NewPtr(discord.PermissionManageChannels),
	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "enable",
			Description: "Enable pace control for a channel",
			Options: []discord.ApplicationCommandOption{
				channelOption,
				discord.ApplicationCommandOptionInt{
					Name:        "target-wpm",
					Description: "Target words per minute (default: 100)",
					Required:    false,
					MinValue:    utils.Ref(10),
					MaxValue:    utils.Ref(1000),
				},
				discord.ApplicationCommandOptionInt{
					Name:        "min-slowmode",
					Description: "Minimum slow mode in seconds (default: 0)",
					Required:    false,
					MinValue:    utils.Ref(0),
					MaxValue:    utils.Ref(120),
				},
				discord.ApplicationCommandOptionInt{
					Name:        "max-slowmode",
					Description: "Maximum slow mode in seconds (default: 30)",
					Required:    false,
					MinValue:    utils.Ref(1),
					MaxValue:    utils.Ref(120),
				},
				discord.ApplicationCommandOptionInt{
					Name:        "activation-wpm",
					Description: "WPM threshold before pace control activates (0 = always active, default: 0)",
					Required:    false,
					MinValue:    utils.Ref(0),
					MaxValue:    utils.Ref(2000),
				},
				discord.ApplicationCommandOptionInt{
					Name:        "wpm-window",
					Description: "Seconds of history to measure WPM over (default: 60)",
					Required:    false,
					MinValue:    utils.Ref(10),
					MaxValue:    utils.Ref(300),
				},
				discord.ApplicationCommandOptionInt{
					Name:        "user-window",
					Description: "Seconds of history to count active users over (default: 120)",
					Required:    false,
					MinValue:    utils.Ref(10),
					MaxValue:    utils.Ref(300),
				},
			},
		},
		discord.ApplicationCommandOptionSubCommand{
			Name:        "disable",
			Description: "Disable pace control for a channel",
			Options: []discord.ApplicationCommandOption{
				channelOption,
			},
		},
		discord.ApplicationCommandOptionSubCommand{
			Name:        "override",
			Description: "Disable pace control and set a static slow mode",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionInt{
					Name:        "slowmode",
					Description: "Static slow mode to set in seconds (0 to remove)",
					Required:    true,
					MinValue:    utils.Ref(0),
					MaxValue:    utils.Ref(21600),
				},
				channelOption,
			},
		},
		discord.ApplicationCommandOptionSubCommand{
			Name:        "status",
			Description: "Show pace control settings",
			Options: []discord.ApplicationCommandOption{
				channelOption,
			},
		},
	},
}

var slowModeCommand = discord.SlashCommandCreate{
	Name:                     "slow-mode",
	Description:              "Set a static slow mode (disables pace control)",
	DefaultMemberPermissions: omit.NewPtr(discord.PermissionManageChannels),
	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionInt{
			Name:        "slowmode",
			Description: "Static slow mode to set in seconds (0 to remove)",
			Required:    true,
			MinValue:    utils.Ref(0),
			MaxValue:    utils.Ref(21600),
		},
		channelOption,
	},
}

// resolveChannel returns the explicitly provided channel or falls back to the
// channel where the interaction was invoked.
func resolveChannel(data discord.SlashCommandInteractionData, e *handler.CommandEvent) snowflake.ID {
	if id, ok := data.OptSnowflake("channel"); ok {
		return id
	}
	return e.Channel().ID()
}

func enableHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("pace-control enable", e)
	data := e.SlashCommandInteractionData()

	guild, inGuild := e.Guild()
	if !inGuild {
		return interactions.ErrEventNoGuildID
	}

	channelID := resolveChannel(data, e)
	targetWPM, hasTarget := data.OptInt("target-wpm")
	if !hasTarget {
		targetWPM = 100
	}
	minSlow, hasMin := data.OptInt("min-slowmode")
	if !hasMin {
		minSlow = 0
	}
	maxSlow, hasMax := data.OptInt("max-slowmode")
	if !hasMax {
		maxSlow = 30
	}
	activationWPM, hasActivation := data.OptInt("activation-wpm")
	if !hasActivation {
		activationWPM = 0
	}
	wpmWindow, hasWPMWindow := data.OptInt("wpm-window")
	if !hasWPMWindow {
		wpmWindow = 60
	}
	userWindow, hasUserWindow := data.OptInt("user-window")
	if !hasUserWindow {
		userWindow = 120
	}

	if minSlow > maxSlow {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"Min slowmode cannot be greater than max slowmode.",
		))
	}

	pc := &model.PaceControl{
		GuildID:           guild.ID,
		ChannelID:         channelID,
		Enabled:           true,
		TargetWPM:         targetWPM,
		MinSlowmode:       minSlow,
		MaxSlowmode:       maxSlow,
		ActivationWPM:     activationWPM,
		WPMWindowSeconds:  wpmWindow,
		UserWindowSeconds: userWindow,
	}

	if err := model.SetPaceControl(pc); err != nil {
		slog.Error("Failed to save pace control settings", "err", err)
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to save settings."))
	}

	activationStr := "always active"
	if activationWPM > 0 {
		activationStr = fmt.Sprintf("%d WPM", activationWPM)
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(
		fmt.Sprintf("Pace control enabled for <#%s>.\nTarget WPM: %d\nActivation: %s\nMin slowmode: %ds\nMax slowmode: %ds\nWPM window: %ds\nUser window: %ds",
			channelID, targetWPM, activationStr, minSlow, maxSlow, wpmWindow, userWindow),
	))
}

func disableHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("pace-control disable", e)
	data := e.SlashCommandInteractionData()

	guild, inGuild := e.Guild()
	if !inGuild {
		return interactions.ErrEventNoGuildID
	}

	channelID := resolveChannel(data, e)

	if err := model.DeletePaceControl(guild.ID, channelID); err != nil {
		slog.Error("Failed to delete pace control settings", "err", err)
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to remove settings."))
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(
		fmt.Sprintf("Pace control disabled for <#%s>.", channelID),
	))
}

func statusHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("pace-control status", e)
	data := e.SlashCommandInteractionData()

	guild, inGuild := e.Guild()
	if !inGuild {
		return interactions.ErrEventNoGuildID
	}

	channelID, hasChannel := data.OptSnowflake("channel")

	if hasChannel {
		pc, err := model.GetPaceControl(guild.ID, channelID)
		if err != nil {
			return e.CreateMessage(interactions.EphemeralMessageContent(
				fmt.Sprintf("No pace control configured for <#%s>.", channelID),
			))
		}
		return e.CreateMessage(interactions.EphemeralMessageContent(formatPaceControl(pc)))
	}

	channels, err := model.GetPaceControlChannels(guild.ID)
	if err != nil || len(channels) == 0 {
		return e.CreateMessage(interactions.EphemeralMessageContent("No pace control channels configured."))
	}

	msg := "## Pace Control Channels\n"
	for i := range channels {
		msg += formatPaceControl(&channels[i]) + "\n"
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(msg))
}

// doOverride is the shared logic for /pace-control override and /slow-mode.
func doOverride(e *handler.CommandEvent, label string) error {
	utils.LogInteraction(label, e)
	data := e.SlashCommandInteractionData()

	guild, inGuild := e.Guild()
	if !inGuild {
		return interactions.ErrEventNoGuildID
	}

	channelID := resolveChannel(data, e)
	slowmode := data.Int("slowmode")

	// Disable pace control if it exists for this channel
	pc, err := model.GetPaceControl(guild.ID, channelID)
	if err == nil && pc.Enabled {
		pc.Enabled = false
		if err := model.SetPaceControl(pc); err != nil {
			slog.Error("Failed to disable pace control during override", "err", err)
		}
	}

	// Set the static slow mode on the channel
	_, err = e.Client().Rest.UpdateChannel(channelID, discord.GuildTextChannelUpdate{
		RateLimitPerUser: utils.Ref(slowmode),
	})
	if err != nil {
		slog.Error("Failed to set slow mode during override", "err", err, "channel", channelID)
		return e.CreateMessage(interactions.EphemeralMessageContent(
			fmt.Sprintf("Failed to set slow mode on <#%s>: %s", channelID, err),
		))
	}

	msg := fmt.Sprintf("Pace control disabled for <#%s>. Slow mode set to %ds.", channelID, slowmode)
	if slowmode == 0 {
		msg = fmt.Sprintf("Pace control disabled for <#%s>. Slow mode removed.", channelID)
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(msg))
}

func overrideHandler(e *handler.CommandEvent) error {
	return doOverride(e, "pace-control override")
}

func slowModeAliasHandler(e *handler.CommandEvent) error {
	return doOverride(e, "slow-mode")
}

func formatPaceControl(pc *model.PaceControl) string {
	activationStr := "always"
	if pc.ActivationWPM > 0 {
		activationStr = fmt.Sprintf("%d WPM", pc.ActivationWPM)
	}
	return fmt.Sprintf(
		"<#%s>: Enabled=%s, Target WPM=%d, Activation=%s, Slowmode=[%d–%d]s, WPM window=%ds, User window=%ds",
		pc.ChannelID,
		utils.Iif(pc.Enabled, "yes", "no"),
		pc.TargetWPM,
		activationStr,
		pc.MinSlowmode,
		pc.MaxSlowmode,
		pc.WPMWindowSeconds,
		pc.UserWindowSeconds,
	)
}

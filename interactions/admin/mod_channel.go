package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var modChannelSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "channels",
	Description: "View or set the moderator channels",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionChannel{
			Name:         "mod-channel",
			Description:  "The channel to set as the moderator channel",
			Required:     false,
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
		},
		discord.ApplicationCommandOptionChannel{
			Name:         "log-channel",
			Description:  "the channel in which various information will be logged",
			Required:     false,
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset the moderator channel setting",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Moderator channel", Value: "mod-channel"},
				{Name: "Log channel", Value: "log-channel"},
				{Name: "All", Value: "all"},
			},
		},
	},
}

func AdminChannelsHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	modChannel, hasModChannel := data.OptChannel("mod-channel")
	logChannel, hasLogChannel := data.OptChannel("log-channel")
	resetOption, hasReset := data.OptString("reset")
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if !hasModChannel && !hasReset && !hasLogChannel {
		return e.CreateMessage(interactions.EphemeralMessageContent(modChannelInfo(settings)).Build())
	}

	message := ""

	if hasReset && resetOption == "mod-channel" {
		switch resetOption {
		case "mod-channel":
			settings.ModeratorChannel = 0
			message += "Moderator channel has been reset.\n"
		case "log-channel":
			settings.LogChannel = 0
			message += "Log channel has been reset.\n"
		case "all":
			settings.ModeratorChannel = 0
			settings.LogChannel = 0
			message += "All channel settings have been reset.\n"
		}

	}

	if hasModChannel {
		settings.ModeratorChannel = modChannel.ID
		message += fmt.Sprintf("Moderator channel set to <#%d>\n", modChannel.ID)
	}
	if hasLogChannel {
		settings.LogChannel = logChannel.ID
		message += fmt.Sprintf("Log channel set to <#%d>\n", logChannel.ID)
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(
		interactions.EphemeralMessageContent(message).
			Build(),
	)
}

func modChannelInfo(settings *model.GuildSettings) string {
	modChannelInfo := "> This is the channel in which notifications and other information for moderators and administrators are sent."
	logChannelInfo := "> This is the channel in which various less-important (and potentially frequent) info will be sent."
	return fmt.Sprintf(
		"## Channels\n**Moderator channel:** %s\n%s\n\n**Log channel:** %s\n%s",
		utils.MentionChannelOrDefault(&settings.ModeratorChannel, "not set"),
		modChannelInfo,
		utils.MentionChannelOrDefault(&settings.LogChannel, "not set"),
		logChannelInfo,
	)
}

package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
)

var modChannelSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "mod-channel",
	Description: "View or set the moderator channel",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionChannel{
			Name:         "channel",
			Description:  "The channel to set as the moderator channel",
			Required:     false,
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset the moderator channel setting",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Reset", Value: "reset"},
			},
		},
	},
}

func AdminModChannelHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	channel, hasChannel := data.OptChannel("channel")
	resetOption, hasReset := data.OptString("reset")
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if !hasChannel && !hasReset {
		return e.CreateMessage(interactions.EphemeralMessageContent(modChannelInfo(settings)).Build())
	}

	message := ""

	if hasReset && resetOption == "reset" {
		settings.ModeratorChannel = 0
		message = "Moderator channel has been reset."
	} else if hasChannel {
		settings.ModeratorChannel = channel.ID
		message = fmt.Sprintf("Moderator channel set to <#%d>", channel.ID)
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
	return fmt.Sprintf(
		"**Moderator channel:** <#%d>\n%s",
		settings.ModeratorChannel, modChannelInfo,
	)
}

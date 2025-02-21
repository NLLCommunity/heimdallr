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
	},
}

func AdminModChannelHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	channel, hasChannel := data.OptChannel("channel")
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if !hasChannel {
		return interactions.RespondWithContentEph(e, modChannelInfo(settings))
	}

	settings.ModeratorChannel = channel.ID
	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return interactions.RespondWithContentEph(e, fmt.Sprintf("Moderator channel set to <#%d>", channel.ID))
}

func modChannelInfo(settings *model.GuildSettings) string {
	modChannelInfo := "> This is the channel in which notifications and other information for moderators and administrators are sent."
	return fmt.Sprintf(
		"**Moderator channel:** <#%d>\n%s",
		settings.ModeratorChannel, modChannelInfo,
	)
}

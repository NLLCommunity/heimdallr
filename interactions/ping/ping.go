package ping

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/ping", PingHandler)

	return []discord.ApplicationCommandCreate{PingCommand}
}

var PingCommand = discord.SlashCommandCreate{
	Name:        "ping",
	Description: "ping",
}

func PingHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ping", e)
	return e.CreateMessage(
		interactions.EphemeralMessageContent("Pong!").
			Build(),
	)
}

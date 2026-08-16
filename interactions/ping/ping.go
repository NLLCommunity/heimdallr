package ping

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r handler.Router) []discord.ApplicationCommandCreate {
	slash := Ping.Register(r)

	return []discord.ApplicationCommandCreate{slash}
}

var Ping = rave.Slash("ping", "Ping the bot").Handle(PingHandler)

func PingHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ping", e)
	return e.CreateMessage(
		interactions.EphemeralMessageContent("Pong!"),
	)
}

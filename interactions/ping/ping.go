package ping

import (
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

var Interactions = rave.Bundle(Ping)

var Ping = rave.Slash("ping", "Ping the bot").Handle(PingHandler)

func PingHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ping", e)
	return e.CreateMessage(
		interactions.EphemeralMessageContent("Pong!"),
	)
}

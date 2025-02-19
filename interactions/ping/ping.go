package ping

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
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
	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Pong!").Build(),
	)
}

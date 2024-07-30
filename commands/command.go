package commands

import (
	"errors"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

type Command struct {
	ApplicationCommand discord.ApplicationCommandCreate
	Handler            func(e *handler.CommandEvent) error
}

type Component struct {
	ComponentPath string
	Handler       func(e *handler.ComponentEvent) error
}

var ErrEventNoGuildID = errors.New("no guild id found in event")

var PingCommand = discord.SlashCommandCreate{
	Name:        "ping",
	Description: "ping",
}

func PingHandler(e *handler.CommandEvent) error {
	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent("test").Build())
}

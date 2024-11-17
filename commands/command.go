package commands

import (
	"errors"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
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

type InteractionEvent interface {
	GuildID() *snowflake.ID
	CreateMessage(message discord.MessageCreate, opts ...rest.RequestOpt) error
	CreateFollowupMessage(message discord.MessageCreate, opts ...rest.RequestOpt) (*discord.Message, error)
}

func CreateMessage(e InteractionEvent, ephemeral bool, message string) error {
	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(ephemeral).
		SetContent(message).Build())
}

func CreateMessagef(e InteractionEvent, ephemeral bool, message string, args ...any) error {
	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(ephemeral).
		SetContentf(message, args...).Build())
}

func CreateFollowupMessage(e InteractionEvent, ephemeral bool, message string) error {
	_, err := e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(ephemeral).
		SetContent(message).Build())
	return err
}

func CreateFollowupMessagef(e InteractionEvent, ephemeral bool, message string, args ...any) error {
	_, err := e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(ephemeral).
		SetContentf(message, args...).Build())
	return err
}

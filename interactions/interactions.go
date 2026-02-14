package interactions

import (
	"errors"
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

type AppCommandRegisterer interface {
	Register(r *handler.Mux) []discord.ApplicationCommandCreate
}

type ApplicationCommandRegisterFunc func(r *handler.Mux) []discord.ApplicationCommandCreate

func (f ApplicationCommandRegisterFunc) Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	return f(r)
}

var ErrEventNoGuildID = errors.New("no guild id found in event")

type DMError struct {
	dmChannelCreated bool
	messageSent      bool
	err              error
}

func (e *DMError) Error() string {
	if !e.dmChannelCreated {
		return "failed to create DM channel"
	}
	if !e.messageSent {
		return "failed to send message"
	}
	return "unknown error"
}

func (e *DMError) Unwrap() error {
	return e.err
}

func NewDMError(dmChannelCreated, messageSent bool, inner error) *DMError {
	return &DMError{
		dmChannelCreated: dmChannelCreated,
		messageSent:      messageSent,
		err:              inner,
	}
}

func SendDirectMessage(client *bot.Client, user discord.User, messageCreate discord.MessageCreate) (
	*discord.Message, error,
) {
	dmChannel, err := client.Rest.CreateDMChannel(user.ID)
	if err != nil {
		return nil, NewDMError(false, false, err)
	}

	if dmChannel == nil {
		return nil, NewDMError(false, false, nil)
	}

	msg, err := client.Rest.CreateMessage(
		dmChannel.ID(),
		messageCreate,
	)
	if err != nil {
		return msg, NewDMError(true, false, err)
	}
	return msg, nil
}

func EphemeralMessageContent(content string) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithContent(content).
		WithEphemeral(true).
		WithAllowedMentions(&discord.AllowedMentions{})
}

func EphemeralMessageContentf(content string, fmtArgs ...any) discord.MessageCreate {
	return EphemeralMessageContent(fmt.Sprintf(content, fmtArgs...))
}

package interactions

import (
	"errors"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
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

func SendDirectMessage(client bot.Client, user discord.User, messageCreate discord.MessageCreate) (
	*discord.Message, error,
) {
	dmChannel, err := client.Rest().CreateDMChannel(user.ID)
	if err != nil {
		return nil, NewDMError(false, false, err)
	}

	if dmChannel == nil {
		return nil, NewDMError(false, false, nil)
	}

	msg, err := client.Rest().CreateMessage(
		dmChannel.ID(),
		messageCreate,
	)
	if err != nil {
		return msg, NewDMError(true, false, err)
	}

	return msg, nil
}
func RespondWithContentEph(e InteractionMessager, content string, fmt ...any) error {
	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetContentf(content, fmt...).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
}

func FollowupWithContentEph(e InteractionMessager, content string, fmt ...any) error {
	_, err := e.CreateFollowupMessage(
		discord.NewMessageCreateBuilder().
			SetContentf(content, fmt...).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build(),
	)
	return err
}

type InteractionMessager interface {
	CreateMessage(messageCreate discord.MessageCreate, opts ...rest.RequestOpt) error
	GetInteractionResponse(opts ...rest.RequestOpt) (*discord.Message, error)
	UpdateInteractionResponse(messageUpdate discord.MessageUpdate, opts ...rest.RequestOpt) (*discord.Message, error)
	DeleteInteractionResponse(opts ...rest.RequestOpt) error
	GetFollowupMessage(messageID snowflake.ID, opts ...rest.RequestOpt) (*discord.Message, error)
	CreateFollowupMessage(messageCreate discord.MessageCreate, opts ...rest.RequestOpt) (*discord.Message, error)
	UpdateFollowupMessage(messageID snowflake.ID, messageUpdate discord.MessageUpdate, opts ...rest.RequestOpt) (*discord.Message, error)
	DeleteFollowupMessage(messageID snowflake.ID, opts ...rest.RequestOpt) error
}

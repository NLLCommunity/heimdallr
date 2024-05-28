package commands

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
)

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

func SendDirectMessage(client bot.Client, user discord.User, messageCreate discord.MessageCreate) (*discord.Message, error) {
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

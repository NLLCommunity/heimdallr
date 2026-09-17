// Package starboard maintains independently scored guild starboards.
package starboard

import (
	"errors"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"time"
)

var ErrNotFound = errors.New("message not found")
var ErrNotSent = errors.New("Discord confirmed the message was not sent")
var ErrAmbiguousSend = errors.New("starboard send is awaiting recovery; refusing to create a duplicate")

type Votes struct{ Positive, Negative []snowflake.ID }

type Transport interface {
	ValidateBoard(model.Starboard) error
	Source(guild, channel, message snowflake.ID) (*discord.Message, error)
	Eligible(model.Starboard, *discord.Message) (bool, snowflake.ID, error)
	Votes(channel, message snowflake.ID, emoji string) (Votes, error)
	Publish(model.Starboard, *discord.Message, Votes, string) (snowflake.ID, error)
	Recover(model.Starboard, string, time.Time) (snowflake.ID, error)
	Update(model.Starboard, *discord.Message, snowflake.ID, Votes) error
	Delete(channel, message snowflake.ID) error
}

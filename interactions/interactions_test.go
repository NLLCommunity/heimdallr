package interactions

import (
	"errors"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewDMError(t *testing.T) {
	innerErr := errors.New("inner error")

	tests := []struct {
		name             string
		dmChannelCreated bool
		messageSent      bool
		innerError       error
		expectedError    string
	}{
		{
			name:             "failed to create DM channel",
			dmChannelCreated: false,
			messageSent:      false,
			innerError:       innerErr,
			expectedError:    "failed to create DM channel",
		},
		{
			name:             "failed to send message",
			dmChannelCreated: true,
			messageSent:      false,
			innerError:       innerErr,
			expectedError:    "failed to send message",
		},
		{
			name:             "unknown error",
			dmChannelCreated: true,
			messageSent:      true,
			innerError:       innerErr,
			expectedError:    "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewDMError(tt.dmChannelCreated, tt.messageSent, tt.innerError)
			assert.Equal(t, tt.expectedError, err.Error())
			assert.Equal(t, tt.innerError, errors.Unwrap(err))
		})
	}
}

func TestEphemeralMessageContent(t *testing.T) {
	content := "Test message"
	builder := EphemeralMessageContent(content)

	message := builder.Build()

	assert.Equal(t, content, message.Content)
	assert.True(t, message.Flags.Has(discord.MessageFlagEphemeral))
	assert.NotNil(t, message.AllowedMentions)
	assert.Empty(t, message.AllowedMentions.Parse)
	assert.Empty(t, message.AllowedMentions.Users)
	assert.Empty(t, message.AllowedMentions.Roles)
}

func TestEphemeralMessageContentf(t *testing.T) {
	template := "User %s has %d points"
	username := "testuser"
	points := 42

	builder := EphemeralMessageContentf(template, username, points)
	message := builder.Build()

	expected := "User testuser has 42 points"
	assert.Equal(t, expected, message.Content)
	assert.True(t, message.Flags.Has(discord.MessageFlagEphemeral))
}

// MockBot is a mock implementation of bot.Client for testing
type MockBot struct {
	mock.Mock
}

func (m *MockBot) Rest() *MockRest {
	args := m.Called()
	return args.Get(0).(*MockRest)
}

type MockRest struct {
	mock.Mock
}

func (m *MockRest) CreateDMChannel(userID snowflake.ID) (discord.DMChannel, error) {
	args := m.Called(userID)
	return args.Get(0).(discord.DMChannel), args.Error(1)
}

func (m *MockRest) CreateMessage(channelID snowflake.ID, messageCreate discord.MessageCreate) (*discord.Message, error) {
	args := m.Called(channelID, messageCreate)
	return args.Get(0).(*discord.Message), args.Error(1)
}

func TestApplicationCommandRegisterFunc(t *testing.T) {
	// Test that ApplicationCommandRegisterFunc implements AppCommandRegisterer.
	var _ AppCommandRegisterer = ApplicationCommandRegisterFunc(nil)

	// Test the Register method.
	expectedCommands := []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:        "test",
			Description: "test command",
		},
	}

	registerFunc := ApplicationCommandRegisterFunc(func(r *handler.Mux) []discord.ApplicationCommandCreate {
		return expectedCommands
	})

	result := registerFunc.Register(nil)
	assert.Equal(t, expectedCommands, result)
}

func TestErrEventNoGuildID(t *testing.T) {
	assert.Equal(t, "no guild id found in event", ErrEventNoGuildID.Error())
}

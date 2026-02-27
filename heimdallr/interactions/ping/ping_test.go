package ping

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/assert"
)

func TestPingCommand(t *testing.T) {
	// Test that the command is properly configured.
	assert.Equal(t, "ping", PingCommand.Name)
	assert.Equal(t, "ping", PingCommand.Description)
}

func TestRegister(t *testing.T) {
	// Create a real handler mux for testing.
	mux := handler.New()

	commands := Register(mux)

	assert.Len(t, commands, 1)
	assert.Equal(t, PingCommand.Name, commands[0].(discord.SlashCommandCreate).Name)
}

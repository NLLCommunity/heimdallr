package quote

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/require"
)

func TestQuoteCommandUsesDefaultIntegrationTypesInGuildContext(t *testing.T) {
	command, ok := Quote.Build().(discord.SlashCommandCreate)
	require.True(t, ok)
	require.Nil(t, command.IntegrationTypes)
	require.Equal(t, []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	}, command.Contexts)
}

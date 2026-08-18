package dashboard

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/stretchr/testify/require"
)

func TestDashboardCommandHasNoDefaultMemberPermissionRestriction(t *testing.T) {
	command := Dashboard.Build().(discord.SlashCommandCreate)

	require.Nil(t, command.DefaultMemberPermissions.Value)
}

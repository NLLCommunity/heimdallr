package role_button

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/require"
)

func TestRegisterInstallsAssignRoleRoute(t *testing.T) {
	router := handler.New()
	Interactions(router)
	require.True(t, router.Match("/role/assign/42", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
}

package infractions

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
)

func TestInfractionRoutesPreserveRegistrationAndCustomIDs(t *testing.T) {
	router := handler.New()
	Register(router)
	require.True(t, router.Match("/infractions-user/5", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	require.True(t, router.Match("/infractions-mod/42/10", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))

	userID, err := userInfractionRoute.CustomID(userInfractionRouteVars{Offset: 5})
	require.NoError(t, err)
	require.Equal(t, "/infractions-user/5", userID)

	moderatorID, err := moderatorInfractionRoute.CustomID(moderatorInfractionRouteVars{
		UserID: snowflake.ID(42),
		Offset: 10,
	})
	require.NoError(t, err)
	require.Equal(t, "/infractions-mod/42/10", moderatorID)
}

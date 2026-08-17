package role_button

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/rave"
)

var assignRoleRoute = rave.Component("/role/assign/{roleID}").
	Handle(RoleAssignButtonHandler)

func Register(r handler.Router) []discord.ApplicationCommandCreate {
	assignRoleRoute.Register(r)
	return []discord.ApplicationCommandCreate{CreateRoleButton.Register(r)}
}

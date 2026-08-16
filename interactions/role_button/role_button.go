package role_button

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

func Register(r handler.Router) []discord.ApplicationCommandCreate {
	r.Component("/role/assign/{roleID}", RoleAssignButtonHandler)

	slash := CreateRoleButton.Register(r)

	return []discord.ApplicationCommandCreate{slash}
}

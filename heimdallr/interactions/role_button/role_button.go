package role_button

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/create-role-button", CreateRoleButtonHandler)
	r.Component("/role/assign/{roleID}", RoleAssignButtonHandler)

	return []discord.ApplicationCommandCreate{CreateRoleButtonCommand}
}

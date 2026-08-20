package role_button

import (
	"github.com/NLLCommunity/heimdallr/rave"
)

var Interactions = rave.Bundle(
	CreateRoleButton,
	assignRoleRoute,
)

var assignRoleRoute = rave.Component("/role/assign/{roleID}").
	Handle(RoleAssignButtonHandler)

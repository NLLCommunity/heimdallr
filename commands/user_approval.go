package commands

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/myrkvi/heimdallr/utils"
)

var ApproveUserCommand = discord.SlashCommandCreate{
	Name:                     "approve-user",
	Description:              "Approve a user to join the server",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	DMPermission:             utils.Ref(false),

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to approve",
			Required:    true,
		},
	},
}

func ApproveUserCommandHandler(e *handler.CommandEvent) {

}

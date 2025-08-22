package gatekeep

import (
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"

	"github.com/NLLCommunity/heimdallr/utils"
)

var ApproveSlashCommand = discord.SlashCommandCreate{
	Name:                     "approve",
	Description:              "Approve a user to join the server",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
	Contexts: []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	},

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to approve",
			Required:    true,
		},
	},
}

func ApproveSlashCommandHandler(e *handler.CommandEvent) error {
	utils.LogInteractionContext("gatekeep", e, e.Ctx)

	guild, success, inGuild := getGuild(e)
	if !inGuild {
		slog.Warn("approve command supplied in DMs or guild ID is otherwise nil")
		return nil
	}
	if !success {
		slog.Warn("approve command: failed to get guild")
		return nil
	}

	member := e.SlashCommandInteractionData().Member("user")

	return approvedInnerHandler(e, guild, member)
}

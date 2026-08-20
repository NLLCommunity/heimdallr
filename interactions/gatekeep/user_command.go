package gatekeep

import (
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

var ApproveUserCommand = rave.UserCommand("Approve").
	WithDefaultMemberPermissions(discord.PermissionKickMembers).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	AddContexts(discord.InteractionContextTypeGuild).
	Handle(ApproveUserCommandHandler)

func ApproveUserCommandHandler(e *handler.CommandEvent) error {
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

	member := e.UserCommandInteractionData().TargetMember()

	return approvedInnerHandler(e, guild, member)
}

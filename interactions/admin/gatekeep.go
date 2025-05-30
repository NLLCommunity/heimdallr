package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var gatekeepSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "gatekeep",
	Description: "View or set gatekeep-related settings",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionBool{
			Name:        "enabled",
			Description: "Whether to enable the gatekeep system",
			Required:    false,
		},
		discord.ApplicationCommandOptionRole{
			Name:        "pending-role",
			Description: "The role to give to users pending approval",
			Required:    false,
		},
		discord.ApplicationCommandOptionRole{
			Name:        "approved-role",
			Description: "The role to give to approved users",
			Required:    false,
		},
		discord.ApplicationCommandOptionBool{
			Name:        "use-pending-role",
			Description: "Whether to give the pending role to users when they join",
			Required:    false,
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset a setting to its default value",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Enabled", Value: "enabled"},
				{Name: "Pending role", Value: "pending-role"},
				{Name: "Approved role", Value: "approved-role"},
				{Name: "Use pending role", Value: "use-pending-role"},
				{Name: "All", Value: "all"},
			},
		},
	},
}

func AdminGatekeepHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin", e)

	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	resetOption, hasReset := data.OptString("reset")
	if hasReset {
		switch resetOption {
		case "enabled":
			settings.GatekeepEnabled = false
			message += "Gatekeep enabled has been reset.\n"
		case "pending-role":
			settings.GatekeepPendingRole = 0
			message += "Gatekeep pending role has been reset.\n"
		case "approved-role":
			settings.GatekeepApprovedRole = 0
			message += "Gatekeep approved role has been reset.\n"
		case "use-pending-role":
			settings.GatekeepAddPendingRoleOnJoin = false
			message += "Give pending role on join has been reset.\n"
		case "all":
			settings.GatekeepEnabled = false
			settings.GatekeepPendingRole = 0
			settings.GatekeepApprovedRole = 0
			settings.GatekeepAddPendingRoleOnJoin = false
			message += "All gatekeep settings have been reset.\n"
		}
	}

	enabled, hasEnabled := data.OptBool("enabled")
	if hasEnabled {
		settings.GatekeepEnabled = enabled
		message += fmt.Sprintf("Gatekeep enabled set to %s\n", utils.Iif(enabled, "yes", "no"))
	}

	pendingRole, hasPendingRole := data.OptRole("pending-role")
	if hasPendingRole {
		settings.GatekeepPendingRole = pendingRole.ID
		message += fmt.Sprintf("Gatekeep pending role set to <@&%d>\n", pendingRole.ID)
	}

	approvedRole, hasApprovedRole := data.OptRole("approved-role")
	if hasApprovedRole {
		settings.GatekeepApprovedRole = approvedRole.ID
		message += fmt.Sprintf("Gatekeep approved role set to <@&%d>\n", approvedRole.ID)
	}

	usePendingRole, hasUsePendingRole := data.OptBool("use-pending-role")
	if hasUsePendingRole {
		settings.GatekeepAddPendingRoleOnJoin = usePendingRole
		message += fmt.Sprintf("Give pending role on join set to %s\n", utils.Iif(usePendingRole, "yes", "no"))
	}

	if !utils.Any(hasEnabled, hasPendingRole, hasApprovedRole, hasUsePendingRole, hasReset) {
		return e.CreateMessage(interactions.EphemeralMessageContent(gatekeepInfo(settings)).Build())
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(message).Build())
}

func gatekeepInfo(settings *model.GuildSettings) string {
	gatekeepEnabledInfo := "> This determines whether to enable the gatekeep system."
	gatekeepEnabled := fmt.Sprintf(
		"**Gatekeep enabled:** %s\n%s",
		utils.Iif(settings.GatekeepEnabled, "yes", "no"), gatekeepEnabledInfo,
	)

	gatekeepPendingRoleInfo := "> This is the role given to users pending approval."
	gatekeepPendingRole := fmt.Sprintf(
		"**Gatekeep pending role:** %s\n%s",
		utils.MentionRoleOrDefault(&settings.GatekeepPendingRole, "not set"),
		gatekeepPendingRoleInfo,
	)

	gatekeepApprovedRoleInfo := "> This is the role given to approved users."
	gatekeepApprovedRole := fmt.Sprintf(
		"**Gatekeep approved role:** %s\n%s",
		utils.MentionRoleOrDefault(&settings.GatekeepApprovedRole, "not set"),
		gatekeepApprovedRoleInfo,
	)

	gatekeepAddPendingRoleOnJoinInfo := "> This determines whether to give the pending role to users when they join."
	gatekeepAddPendingRoleOnJoin := fmt.Sprintf(
		"**Give pending role on join:** %s\n%s",
		utils.Iif(settings.GatekeepAddPendingRoleOnJoin, "yes", "no"), gatekeepAddPendingRoleOnJoinInfo,
	)

	gatekeepApprovedMessageInfo := "Approved message can be viewed by using the `/admin gatekeep-message` command."

	return fmt.Sprintf(
		"## Gatekeep settings\n%s\n\n%s\n\n%s\n\n%s\n\n*%s*",
		gatekeepEnabled, gatekeepPendingRole, gatekeepApprovedRole, gatekeepAddPendingRoleOnJoin,
		gatekeepApprovedMessageInfo,
	)
}

package role_button

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

var CreateRoleButtonCommand = discord.SlashCommandCreate{
	Name: "create-role-button",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "lag-rolleknapp",
	},
	Description: "Create a button that assigns a role to the user when clicked.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Lag ein knapp som gjev brukaren ei rolle når han vert trykt på.",
	},

	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	DefaultMemberPermissions: omit.NewPtr(discord.PermissionManageRoles),
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionRole{
			Name: "role",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "rolle",
			},
			Description: "The role to assign to the user.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Rollen som skal gjevast til brukaren.",
			},
			Required: true,
		},
		discord.ApplicationCommandOptionString{
			Name: "instructions",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "instruksjonar",
			},
			Description: "Instructions to display to the user above the button.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Instruksjonar som skal visast til brukaren over knappen.",
			},
			Required: false,
		},
		discord.ApplicationCommandOptionString{
			Name: "text",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "tekst",
			},
			Description: "The text to display on the button.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Teksten som skal visast på knappen.",
			},
			Required: false,
		},
	},
}

func CreateRoleButtonHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("role button", e)

	if e.GuildID() == nil {
		slog.Warn("Received create role button command in DMs or guild ID is otherwise nil")
		return interactions.ErrEventNoGuildID
	}

	slog.Info(
		"Received create role button command",
		"guildID",
		e.GuildID(),
		"user",
		e.User().ID,
	)

	permissions := e.Member().Permissions

	role := e.SlashCommandInteractionData().Role("role")

	// Local permission checks first, before the hierarchy check below, which
	// costs a GetRoles REST call - an unauthorized caller should be rejected
	// without paying for that round-trip.

	// Check if the user has permission to assign roles
	if !permissions.Has(discord.PermissionManageRoles) {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"You need the Manage Roles permission to create a role button.",
			),
		)
	}

	// Check if the specific role in question is one the user can assign
	if !permissions.Has(role.Permissions) {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"You cannot assign a role with permissions you do not have.",
			),
		)
	}

	hasSufficientRole, err := memberHasRolesAboveTargetRole(e.Client().Rest, *e.GuildID(), e.Member().Member, role)
	if err != nil {
		slog.Error("Failed to check if member has roles above target role", "err", err)
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"An error occurred.",
			),
		)
	}

	if !hasSufficientRole {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"You cannot assign a role that is higher than or equal to your highest role.",
			),
		)
	}

	instructions := e.SlashCommandInteractionData().String("instructions")
	if instructions == "" {
		instructions = fmt.Sprintf("Click the button below to get the **%s** role.", role.Name)
	}

	text := e.SlashCommandInteractionData().String("text")
	if text == "" {
		text = "Get role"
	}

	// Create the button

	return e.CreateMessage(
		discord.NewMessageCreate().
			WithContent(instructions).
			AddActionRow(discord.NewPrimaryButton(text, fmt.Sprintf("/role/assign/%s", role.ID.String()))).
			WithAllowedMentions(&discord.AllowedMentions{}),
	)
}

func memberHasRolesAboveTargetRole(r rest.Rest, guildID snowflake.ID, member discord.Member, targetRole discord.Role) (bool, error) {
	guildRolesPositions, err := getRolesPositionMap(r, guildID)
	if err != nil {
		return false, err
	}

	highestRoleNum := 0
	for _, roleID := range member.RoleIDs {
		if pos, ok := guildRolesPositions[roleID]; ok && pos > highestRoleNum {
			highestRoleNum = pos
		}
	}

	if highestRoleNum <= targetRole.Position {
		return false, nil
	}

	return true, nil
}

func getRolesPositionMap(r rest.Rest, guildID snowflake.ID) (map[snowflake.ID]int, error) {

	guildRoles, err := r.GetRoles(guildID)
	if err != nil {
		return nil, err
	}
	guildToPositionMap := make(map[snowflake.ID]int)
	for _, r := range guildRoles {
		guildToPositionMap[r.ID] = r.Position
	}

	return guildToPositionMap, nil
}

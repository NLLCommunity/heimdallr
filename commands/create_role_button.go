package commands

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"

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

	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionManageRoles),

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
	utils.LogInteraction("create-role-button", e)

	if e.GuildID() == nil {
		slog.Warn("Received create role button command in DMs or guild ID is otherwise nil")
		return ErrEventNoGuildID
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

	instructions := e.SlashCommandInteractionData().String("instructions")
	if instructions == "" {
		instructions = fmt.Sprintf("Click the button below to get the **%s** role.", role.Name)
	}

	text := e.SlashCommandInteractionData().String("text")
	if text == "" {
		text = "Get role"
	}

	// Check if the user has permission to assign roles
	if !permissions.Has(discord.PermissionManageRoles) {
		_ = respondWithContentEph(e, "You need the Manage Roles permission to create a role button.")
		return nil
	}

	// Check if the specific role in question is one the user can assign
	if !permissions.Has(role.Permissions) {
		_ = respondWithContentEph(e, "You cannot assign a role with permissions you do not have.")
		return nil
	}

	// Create the button

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(instructions).
		AddActionRow(discord.NewPrimaryButton(text, fmt.Sprintf("/role/assign/%s", role.ID.String()))).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

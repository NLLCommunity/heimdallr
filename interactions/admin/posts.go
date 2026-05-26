package admin

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var postsSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "posts",
	Description: "View or set posts dashboard settings",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionRole{
			Name:        "mod-role",
			Description: "Role allowed to manage posts in the dashboard (admins always have access)",
			Required:    false,
		},
		discord.ApplicationCommandOptionString{
			Name:        "reset",
			Description: "Reset a setting to its default value",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Mod role", Value: "mod-role"},
				{Name: "All", Value: "all"},
			},
		},
	},
}

func AdminPostsHandler(e *handler.CommandEvent) error {
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
		case "mod-role", "all":
			settings.PostsModRoleID = 0
			message += "Posts mod role has been reset.\n"
		}
	}

	modRole, hasModRole := data.OptRole("mod-role")
	if hasModRole {
		settings.PostsModRoleID = modRole.ID
		message += fmt.Sprintf("Posts mod role set to <@&%d>\n", modRole.ID)
	}

	if !utils.Any(hasModRole, hasReset) {
		return e.CreateMessage(interactions.EphemeralMessageContent(postsInfo(settings)))
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}
	logSettingsCommandUpdate(guild.ID, e.User(), "posts", map[string]any{
		"mod_role": settings.PostsModRoleID.String(),
	})

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

func postsInfo(settings *model.GuildSettings) string {
	modRoleInfo := "> Members with this role can manage posts in the dashboard. Admins always have access regardless of this setting. When unset, only admins can manage posts."
	modRole := fmt.Sprintf(
		"**Posts mod role:** %s\n%s",
		utils.MentionRoleOrDefault(&settings.PostsModRoleID, "not set"),
		modRoleInfo,
	)

	return fmt.Sprintf("## Posts settings\n%s", modRole)
}

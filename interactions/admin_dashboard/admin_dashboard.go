package admin_dashboard

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
	"github.com/spf13/viper"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/admin-dashboard", AdminDashboardHandler)

	return []discord.ApplicationCommandCreate{AdminDashboardCommand}
}

var AdminDashboardCommand = discord.SlashCommandCreate{
	Name:                     "admin-dashboard",
	Description:              "Get a login link for the web dashboard",
	DefaultMemberPermissions: omit.NewPtr(discord.PermissionAdministrator),
	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
}

func AdminDashboardHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("admin-dashboard", e)

	user := e.User()

	var avatar string
	if user.Avatar != nil {
		avatar = *user.Avatar
	}

	code, err := model.CreateLoginCode(user.ID, user.Username, avatar)
	if err != nil {
		return e.CreateMessage(
			interactions.EphemeralMessageContent("Failed to generate login link. Please try again."),
		)
	}

	baseURL := viper.GetString("dashboard.base_url")
	link := fmt.Sprintf("%s/#/callback?code=%s", baseURL, code)

	message := fmt.Sprintf(
		"**Dashboard Login Link**\n\n"+
			"[Click here to open the dashboard](%s)\n\n"+
			"This link expires in **5 minutes** and can only be used once.",
		link,
	)

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

package admin_dashboard

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"

	"github.com/NLLCommunity/heimdallr/config"
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

	// Validate the dashboard URL before consuming a login code, so a misconfig
	// doesn't waste codes and so the operator gets a clear log line. The
	// helper rejects empty/relative values that url.Parse would otherwise
	// accept, which would render as a broken Discord link.
	u, err := config.ParsedDashboardBaseURL()
	if err != nil {
		slog.Error("admin-dashboard: dashboard.base_url is misconfigured", "err", err)
		return e.CreateMessage(
			interactions.EphemeralMessageContent("Dashboard URL is misconfigured. Contact the bot operator."),
		)
	}

	code, err := model.CreateLoginCode(user.ID, user.Username, avatar, "admin", 0)
	if err != nil {
		return e.CreateMessage(
			interactions.EphemeralMessageContent("Failed to generate login link. Please try again."),
		)
	}

	u = u.JoinPath("callback")
	q := u.Query()
	q.Set("code", code)
	u.RawQuery = q.Encode()
	link := u.String()

	message := fmt.Sprintf(
		"**Dashboard Login Link**\n\n"+
			"[Click here to open the dashboard](%s)\n\n"+
			"This link expires in **5 minutes** and can only be used once.",
		link,
	)

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

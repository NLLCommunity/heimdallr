package dashboard

import (
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/config"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

// Register installs /dashboard. The command is a thin deep-link helper:
// authentication itself happens via Discord OAuth on the web side, so the
// command stays open to everyone in the guild and the dashboard enforces
// per-page access via OAuth + the configured PostsModRoleID setting.
func Register(r handler.Router) []discord.ApplicationCommandCreate {
	slash := Dashboard.Register(r)
	return []discord.ApplicationCommandCreate{slash}
}

var Dashboard = rave.Slash("dashboard", "Get a link to the web dashboard").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionManageChannels).
	Handle(Handler)

func Handler(e *handler.CommandEvent) error {
	utils.LogInteraction("dashboard", e)

	u, err := config.ParsedDashboardBaseURL()
	if err != nil {
		slog.Error("dashboard: dashboard.base_url is misconfigured", "err", err)
		return e.CreateMessage(
			interactions.EphemeralMessageContent("Dashboard URL is misconfigured. Contact the bot operator."),
		)
	}

	if gid := e.GuildID(); gid != nil {
		u = u.JoinPath("guild", gid.String())
	}

	return e.CreateMessage(
		interactions.EphemeralMessageContent("Click tke below button to open the dashboard").
			AddActionRow(
				discord.NewLinkButton("Open Dashboard", u.String()).
					WithEmoji(discord.NewComponentEmoji("⚙️")),
			))
}

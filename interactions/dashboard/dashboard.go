package dashboard

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/config"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

// Register installs /dashboard. The command is a thin deep-link helper:
// authentication itself happens via Discord OAuth on the web side, so the
// command stays open to everyone in the guild and the dashboard enforces
// per-page access via OAuth + the configured PostsModRoleID setting.
func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/dashboard", Handler)
	return []discord.ApplicationCommandCreate{Command}
}

var Command = discord.SlashCommandCreate{
	Name:             "dashboard",
	Description:      "Get a link to the web dashboard",
	Contexts:         []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes: []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
}

// Handler builds a deep-link to the dashboard for the current guild and
// returns it ephemerally. Used in a guild context the link points at
// /guild/{guildID}; outside one (impossible at present because Contexts
// limits invocation to guilds, but kept defensive) it points at the
// dashboard root.
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

	message := fmt.Sprintf(
		"**Dashboard**\n\n"+
			"[Open the dashboard](%s)\n\n"+
			"You'll be asked to authorize with Discord on first visit.",
		u.String(),
	)

	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

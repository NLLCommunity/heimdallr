package post_dashboard

import (
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/config"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

// commandID is captured the first time Discord notifies us of an invocation
// (the CommandEvent carries the registered command's ID). The web handlers
// need this ID to fetch per-guild permission overrides — keeping it here as
// an atomic snowflake avoids a global registry.
var commandID atomic.Uint64

// CommandID returns the ID Discord assigned to /post-dashboard, or 0 if the
// command hasn't been invoked yet. Used by web/post_perms.go.
func CommandID() snowflake.ID {
	return snowflake.ID(commandID.Load())
}

// SetCommandID is called by main.go after handler.SyncCommands returns the
// list of registered commands, so we know /post-dashboard's ID without
// having to wait for someone to invoke the command first.
func SetCommandID(cmds []discord.ApplicationCommand) {
	for _, c := range cmds {
		if c.Name() == "post-dashboard" {
			commandID.Store(uint64(c.ID()))
			return
		}
	}
}

// DefaultMemberPerm is the command's default permission gate; web handlers
// use this when no per-guild overrides apply.
const DefaultMemberPerm = discord.PermissionManageMessages

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/post-dashboard", PostDashboardHandler)
	return []discord.ApplicationCommandCreate{PostDashboardCommand}
}

var PostDashboardCommand = discord.SlashCommandCreate{
	Name:                     "post-dashboard",
	Description:              "Open the post-management dashboard.",
	DefaultMemberPermissions: omit.NewPtr(DefaultMemberPerm),
	Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
}

func PostDashboardHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("post-dashboard", e)
	commandID.Store(uint64(e.Data.CommandID()))

	user := e.User()
	if e.GuildID() == nil {
		return e.CreateMessage(interactions.EphemeralMessageContent("This command can only be used in a server."))
	}
	guildID := *e.GuildID()

	var avatar string
	if user.Avatar != nil {
		avatar = *user.Avatar
	}

	u, err := config.ParsedDashboardBaseURL()
	if err != nil {
		slog.Error("post-dashboard: dashboard.base_url is misconfigured", "err", err)
		return e.CreateMessage(interactions.EphemeralMessageContent("Dashboard URL is misconfigured. Contact the bot operator."))
	}

	code, err := model.CreateLoginCode(user.ID, user.Username, avatar, "posts", guildID)
	if err != nil {
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to generate login link. Please try again."))
	}

	u = u.JoinPath("callback")
	q := u.Query()
	q.Set("code", code)
	u.RawQuery = q.Encode()

	message := fmt.Sprintf(
		"**Post Dashboard Login Link**\n\n"+
			"[Click here to open the post dashboard](%s)\n\n"+
			"This link expires in **5 minutes** and can only be used once.",
		u.String(),
	)
	return e.CreateMessage(interactions.EphemeralMessageContent(message))
}

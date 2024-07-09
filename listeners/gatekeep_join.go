package listeners

import (
	"github.com/disgoorg/disgo/events"
	"github.com/myrkvi/heimdallr/model"
	"log/slog"
)

func OnGatekeepUserJoin(e *events.GuildMemberJoin) {
	settings, err := model.GetGuildSettings(e.GuildID)
	if err != nil {
		return
	}

	if !settings.GatekeepAddPendingRoleOnJoin || settings.GatekeepPendingRole == 0 {
		return
	}

	err = e.Client().Rest().AddMemberRole(e.GuildID, e.Member.User.ID, settings.GatekeepPendingRole)
	if err != nil {
		slog.Warn("failed to add pending role to user",
			"guild_id", e.GuildID,
			"user_id", e.Member.User.ID,
			"role_id", settings.GatekeepPendingRole)
	}
}

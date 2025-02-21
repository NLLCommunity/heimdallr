package role_button

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
)

func RoleAssignButtonHandler(e *handler.ComponentEvent) error {
	if e.GuildID() == nil {
		slog.Warn("Received role assign button interaction in DMs or guild ID is otherwise nil")
		return nil
	}

	slog.Info(
		"Received role assign button interaction",
		"guildID",
		e.GuildID(),
		"user",
		e.User().ID,
		"roleID",
		e.Vars["roleID"],
	)

	roleID, err := snowflake.Parse(e.Vars["roleID"])
	if err != nil {
		slog.Warn("Failed to parse roleID", "roleID", e.Vars["roleID"], "err", err)
		return nil
	}

	customID := e.ButtonInteractionData().CustomID()
	comp := e.Message.ComponentByID(customID)
	componentLabel := "role button"
	if comp != nil {
		switch x := comp.(type) {
		case discord.ButtonComponent:
			componentLabel = fmt.Sprintf("role button \"%s\"", x.Label)
		}
	}

	err = e.Client().Rest().AddMemberRole(
		*e.GuildID(), e.User().ID, roleID,
		rest.WithReason(fmt.Sprintf("User pressed %s in channel \"%s\"", componentLabel, e.Channel().Name())),
	)

	if err != nil {
		_ = interactions.MessageEphWithContentf(
			e, "Failed to assign role. This is likely due to the bot not having the required permissions.",
		)
		return err
	}

	return interactions.MessageEphWithContentf(e, "Role assigned!")
}

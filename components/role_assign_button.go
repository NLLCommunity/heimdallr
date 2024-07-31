package components

import (
	_ "embed"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
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
		e.Variables["roleID"],
	)

	roleID, err := snowflake.Parse(e.Variables["roleID"])
	if err != nil {
		slog.Warn("Failed to parse roleID", "roleID", e.Variables["roleID"], "err", err)
		return nil
	}

	err = e.Client().Rest().AddMemberRole(*e.GuildID(), e.User().ID, roleID)

	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent("Role assigned!").
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

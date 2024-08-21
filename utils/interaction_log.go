package utils

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
)

func LogInteraction(interactionName string, interaction discord.Interaction) {
	delay := time.Since(interaction.ID().Time())
	type_ := "unknown"
	switch interaction.Type() {
	case discord.InteractionTypeApplicationCommand:
		type_ = "application command"
	case discord.InteractionTypeComponent:
		type_ = "message component"
	case discord.InteractionTypeModalSubmit:
		type_ = "modal submit"
	case discord.InteractionTypeAutocomplete:
		type_ = "autocomplete"
	}

	slog.Info(fmt.Sprintf("Interaction %s (%s) received", interactionName, type_),
		"user_id", interaction.User().ID,
		"guild_id", interaction.GuildID(),
		"channel_id", interaction.ChannelID(),
		"delay", delay,
	)
}

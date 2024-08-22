package utils

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
)

func LogInteraction(interactionName string, interaction discord.Interaction) {
	delay := time.Since(interaction.ID().Time())
	type_ := getInteractionName(interaction)

	slog.Info(fmt.Sprintf("Interaction %s (%s) received", interactionName, type_),
		"user_id", interaction.User().ID,
		"guild_id", interaction.GuildID(),
		"channel_id", interaction.Channel().ID(),
		"delay", delay,
	)
}

func LogInteractionContext(interactionName string, interaction discord.Interaction, ctx context.Context) {
	delay := time.Since(interaction.ID().Time())
	type_ := getInteractionName(interaction)

	slog.InfoContext(ctx, fmt.Sprintf("Interaction %s (%s) received", interactionName, type_),
		"user_id", interaction.User().ID,
		"guild_id", interaction.GuildID(),
		"channel_id", interaction.Channel().ID(),
		"delay", delay,
	)
}

func getInteractionName(interaction discord.Interaction) string {
	switch interaction.Type() {
	case discord.InteractionTypeApplicationCommand:
		return "application command"
	case discord.InteractionTypeComponent:
		return "message component"
	case discord.InteractionTypeModalSubmit:
		return "modal submit"
	case discord.InteractionTypeAutocomplete:
		return "autocomplete"
	default:
		return "unknown"
	}
}

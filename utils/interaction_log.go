package utils

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

func LogInteraction(namespace string, interaction discord.Interaction) {
	ctx := context.Background()
	LogInteractionContext(namespace, interaction, ctx)
}

func LogInteractionContext(namespace string, interaction discord.Interaction, ctx context.Context) {
	delay := time.Since(interaction.ID().Time())
	type_ := getInteractionName(interaction)

	logAttrs := []any{
		slog.Any("type", type_),
		slog.Any("user_id", interaction.User().ID),
		slog.Any("guild_id", interaction.GuildID()),
		slog.Any("channel_id", interaction.Channel().ID()),
		slog.Any("delay", delay),
	}
	if ix, ok := interaction.(*handler.CommandEvent); ok {
		logAttrs = append(logAttrs, slog.Any("command_name", getCommandName(ix.Data)))
	}

	slog.InfoContext(
		ctx, fmt.Sprintf("Interaction [%s] received", namespace),
		logAttrs...,
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

func getCommandName(data discord.ApplicationCommandInteractionData) string {
	switch data := data.(type) {
	case discord.SlashCommandInteractionData:
		cmd := data.CommandName()

		if data.SubCommandGroupName != nil {
			cmd += " " + *data.SubCommandGroupName
		}

		if data.SubCommandName != nil {
			cmd += " " + *data.SubCommandName
		}

		return cmd

	default:
		return data.CommandName()
	}
}

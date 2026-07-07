package interactions

import (
	"context"
	"log/slog"

	"github.com/NLLCommunity/heimdallr/telemetry"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"go.opentelemetry.io/otel/attribute"
)

// RecoverGo runs the next handler in a goroutine, recovering any panic so that a
// single faulty interaction handler cannot crash the whole bot.
//
// It replaces disgo's middleware.Go, which spawns the handler in a bare
// goroutine. A panic there escapes the event manager's recover and terminates
// the process, taking down every guild and skipping graceful shutdown. Returned
// errors are logged the same way middleware.Go logs them.
func RecoverGo(next handler.Handler) handler.Handler {
	return func(event *handler.InteractionEvent) error {
		go func() {
			ctx := context.Background()
			if event != nil && event.Ctx != nil {
				ctx = event.Ctx
			}

			interactionType, commandName, guildID, channelID, userID := interactionTelemetryFields(event)
			ctx, span := telemetry.StartDefaultSpan(ctx, "discord.interaction", interactionSpanAttributes(
				interactionType,
				commandName,
				guildID,
				channelID,
				userID,
			)...)
			defer span.End()

			if event != nil {
				event.Ctx = ctx
			}

			defer func() {
				if r := recover(); r != nil {
					fields := append(interactionLogFields(interactionType, commandName, guildID, channelID, userID), "panic_recovered", true, "outcome", "panic")
					interactionLogger(event).ErrorContext(
						ctx,
						"recovered from panic in interaction handler",
						fields...,
					)
				}
			}()
			if userID != "" {
				telemetry.Capture(ctx, telemetry.Event{
					Name:       "command_used",
					DistinctID: userID,
					GuildID:    guildID,
					Properties: commandUsedProperties("discord", interactionType, commandName, guildID, channelID),
				})
			}
			if err := next(event); err != nil {
				fields := append(interactionLogFields(interactionType, commandName, guildID, channelID, userID), "error", true, "outcome", "error")
				interactionLogger(event).ErrorContext(ctx, "failed to handle interaction", fields...)
			}
		}()
		return nil
	}
}

func commandUsedProperties(namespace, interactionType, commandName, guildID, channelID string) map[string]any {
	properties := map[string]any{}

	if namespace = telemetry.NormalizeTelemetryToken(namespace); namespace != "" {
		properties["namespace"] = namespace
	}
	if interactionType = normalizeInteractionTypeToken(interactionType); interactionType != "" {
		properties["interaction_type"] = interactionType
	}
	if commandName = telemetry.NormalizeTelemetryToken(commandName); commandName != "" {
		properties["command_name"] = commandName
	}
	if guildID != "" {
		properties["guild_id"] = guildID
	}
	if channelID != "" {
		properties["channel_id"] = channelID
	}
	if len(properties) == 0 {
		return nil
	}
	return properties
}

func interactionTelemetryFields(event *handler.InteractionEvent) (interactionType, commandName, guildID, channelID, userID string) {
	interactionType = "unknown"
	if event == nil || event.Interaction == nil {
		return interactionType, "", "", "", ""
	}

	interactionType = interactionTypeToken(event.Interaction)
	commandName = interactionCommandToken(event.Interaction)

	if user := event.User(); user.ID != 0 {
		userID = user.ID.String()
	}
	if guild := event.GuildID(); guild != nil {
		guildID = guild.String()
	}
	if channel := event.Channel(); channel.MessageChannel != nil && channel.ID() != 0 {
		channelID = channel.ID().String()
	}

	return interactionType, commandName, guildID, channelID, userID
}

func interactionSpanAttributes(interactionType, commandName, guildID, channelID, userID string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 5)
	if interactionType != "" {
		attrs = append(attrs, attribute.String("discord.interaction_type", interactionType))
	}
	if guildID != "" {
		attrs = append(attrs, attribute.String("discord.guild_id", guildID))
	}
	if channelID != "" {
		attrs = append(attrs, attribute.String("discord.channel_id", channelID))
	}
	if userID != "" {
		attrs = append(attrs, attribute.String("discord.user_id", userID))
	}
	if commandName != "" {
		attrs = append(attrs, attribute.String("discord.command_name", commandName))
	}
	return attrs
}

func interactionLogFields(interactionType, commandName, guildID, channelID, userID string) []any {
	fields := make([]any, 0, 10)
	if interactionType != "" {
		fields = append(fields, "interaction_type", interactionType)
	}
	if commandName != "" {
		fields = append(fields, "command_name", commandName)
	}
	if guildID != "" {
		fields = append(fields, "guild_id", guildID)
	}
	if channelID != "" {
		fields = append(fields, "channel_id", channelID)
	}
	if userID != "" {
		fields = append(fields, "user_id", userID)
	}
	return fields
}

func interactionTypeToken(interaction discord.Interaction) string {
	if interaction == nil {
		return "unknown"
	}

	switch interaction.Type() {
	case discord.InteractionTypeApplicationCommand:
		return "application_command"
	case discord.InteractionTypeComponent:
		return "message_component"
	case discord.InteractionTypeAutocomplete:
		return "autocomplete"
	case discord.InteractionTypeModalSubmit:
		return "modal_submit"
	case discord.InteractionTypePing:
		return "ping"
	default:
		return "unknown"
	}
}

func normalizeInteractionTypeToken(value string) string {
	switch telemetry.NormalizeTelemetryToken(value) {
	case "application_command", "slash_command":
		return "application_command"
	case "message_component", "component":
		return "message_component"
	case "autocomplete":
		return "autocomplete"
	case "modal_submit":
		return "modal_submit"
	case "ping":
		return "ping"
	case "unknown":
		return "unknown"
	default:
		return telemetry.NormalizeTelemetryToken(value)
	}
}

func interactionCommandToken(interaction discord.Interaction) string {
	switch interaction := interaction.(type) {
	case discord.ApplicationCommandInteraction:
		if interaction.Data == nil {
			return ""
		}
		return applicationCommandToken(interaction.Data)
	case *discord.ApplicationCommandInteraction:
		if interaction == nil || interaction.Data == nil {
			return ""
		}
		return applicationCommandToken(interaction.Data)
	case discord.AutocompleteInteraction:
		return normalizeCommandParts(
			interaction.Data.CommandName,
			interaction.Data.SubCommandGroupName,
			interaction.Data.SubCommandName,
		)
	case *discord.AutocompleteInteraction:
		if interaction == nil {
			return ""
		}
		return normalizeCommandParts(
			interaction.Data.CommandName,
			interaction.Data.SubCommandGroupName,
			interaction.Data.SubCommandName,
		)
	default:
		return ""
	}
}

func applicationCommandToken(data discord.ApplicationCommandInteractionData) string {
	switch data := data.(type) {
	case discord.SlashCommandInteractionData:
		return normalizeCommandParts(data.CommandName(), data.SubCommandGroupName, data.SubCommandName)
	default:
		return telemetry.NormalizeTelemetryToken(data.CommandName())
	}
}

func normalizeCommandParts(commandName string, parts ...*string) string {
	tokens := make([]string, 0, len(parts)+1)
	if token := telemetry.NormalizeTelemetryToken(commandName); token != "" {
		tokens = append(tokens, token)
	}
	for _, part := range parts {
		if part == nil {
			continue
		}
		if token := telemetry.NormalizeTelemetryToken(*part); token != "" {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 {
		return ""
	}

	joined := tokens[0]
	if len(tokens) > 1 {
		joined += "_" + joinCommandParts(tokens[1:])
	}
	return telemetry.NormalizeTelemetryToken(joined)
}

func joinCommandParts(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	joined := tokens[0]
	for _, token := range tokens[1:] {
		joined += "_" + token
	}
	return joined
}

func interactionLogger(event *handler.InteractionEvent) *slog.Logger {
	if event != nil && event.InteractionCreate != nil && event.GenericEvent != nil {
		if client := event.Client(); client != nil && client.Logger != nil {
			return client.Logger
		}
	}
	return slog.Default()
}

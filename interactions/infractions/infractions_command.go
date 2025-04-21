package infractions

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

// InfractionsCommand is a set of subcommands to manage infractions.
var InfractionsCommand = discord.SlashCommandCreate{
	Name: "infractions",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "advarsler",
	},
	Description: "View a user's warnings.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Se en brukers advarsler.",
	},

	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name: "list",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "liste",
			},
			Description: "View a user's warnings. (NB: Response visible to all)",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Se en brukers advarsler.",
			},
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionUser{
					Name: "user",
					NameLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "bruker",
					},
					Description: "The user to view warnings for.",
					DescriptionLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "Brukeren du vil se advarsler for.",
					},
					Required: false,
				},
				discord.ApplicationCommandOptionString{
					Name: "user-id",
					NameLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "bruker-id",
					},
					Description: "The ID of the user user to view warnings for.",
					DescriptionLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "ID-en til brukeren du vil se advarsler for.",
					},
					Required: false,
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name: "remove",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "fjern",
			},
			Description: "Remove a user's warning.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Fjern en brukers advarsel.",
			},
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name: "infraction-id",
					NameLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "advarsels-id",
					},
					Description: "The id of the infraction to remove.",
					DescriptionLocalizations: map[discord.Locale]string{
						discord.LocaleNorwegian: "ID-en til advarselen du vil fjerne.",
					},
					Required: true,
				},
			},
		},
	},
}

// InfractionsListHandler handles the `/infractions list` command.
func InfractionsListHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("infractions", e)

	slog.Info("interaction `/infractions list` called.")
	data := e.SlashCommandInteractionData()
	user, hasUser := data.OptUser("user")
	userIDString, hasUserID := data.OptString("user-id")

	if !hasUser && !hasUserID {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"You must specify either a user or a user ID.").Build())
	}

	if hasUser && hasUserID {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"You can only specify either a user or a user ID.").Build())
	}

	if !hasUser {
		userID, err := snowflake.Parse(userIDString)
		if err != nil {
			_ = e.CreateMessage(interactions.EphemeralMessageContent(
				"Failed to parse user id.").Build())
			return fmt.Errorf("failed to parse user id: %w", err)
		}

		userRef, err := e.Client().Rest().GetUser(userID)
		if err != nil || userRef == nil {
			user = discord.User{
				ID:       userID,
				Username: "unknown_user",
			}
		} else {
			user = *userRef
		}
	}
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return interactions.ErrEventNoGuildID
	}

	message, err := getUserInfractionsAndMakeMessage(true, &guild, &user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}

	return e.CreateMessage(message.Build())
}

func InfractionsRemoveHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("infractions", e)

	data := e.SlashCommandInteractionData()
	infID := data.String("infraction-id")
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return interactions.ErrEventNoGuildID
	}

	err := model.DeleteInfractionBySqid(infID)
	if err != nil {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"Failed to delete infraction.").Build())
	}

	return e.CreateMessage(interactions.EphemeralMessageContent(
		"Infraction deleted.").Build())
}

func InfractionsListComponentHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("infractions", e)

	parentIx := e.Message.Interaction
	if parentIx == nil {
		return fmt.Errorf("no parent interaction found")
	}

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}
	offset, err := strconv.Atoi(e.Vars["offset"])
	if err != nil {
		return fmt.Errorf("failed to parse offset: %w", err)
	}
	userID, err := snowflake.Parse(e.Vars["userID"])
	if err != nil {
		return fmt.Errorf("failed to parse user id: %w", err)
	}

	user, err := e.Client().Rest().GetUser(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if e.User().ID != parentIx.User.ID {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"You can only paginate responses from your own commands.").Build())
	}

	mcb, mub, err := getUserInfractionsAndUpdateMessage(false, offset, &guild, user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}
	if mcb != nil {
		return e.CreateMessage(mcb.Build())
	}
	return e.UpdateMessage(mub.Build())
}

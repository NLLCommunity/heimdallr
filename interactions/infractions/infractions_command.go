package infractions

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

var Infractions = rave.Slash("infractions", "View a user's infractions").
	AddNameLocalization(discord.LocaleNorwegian, "advarsler").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Se en brukers advarsler.").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionKickMembers).
	AddOptions(

		rave.SubCommand("list", "View a user's warnings. (NB: Response visible to all)").
			Handle(InfractionsListHandler).
			AddNameLocalization(discord.LocaleNorwegian, "liste").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Se en brukers advarsler.").
			AddOptions(

				rave.OptionUser("user", "The user to view warnings for.").
					AddNameLocalization(discord.LocaleNorwegian, "bruker").
					AddDescriptionLocalization(discord.LocaleNorwegian, "Brukeren du vil se advarsler for.").
					WithRequired(true)),

		rave.SubCommand("remove", "Remove a user's warning.").
			Handle(InfractionsRemoveHandler).
			AddNameLocalization(discord.LocaleNorwegian, "fjern").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Fjern en brukers advarsel.").
			AddOptions(

				rave.OptionString("infraction-id", "The id of the infraction to remove.").
					AddNameLocalization(discord.LocaleNorwegian, "advarsels-id").
					AddDescriptionLocalization(discord.LocaleNorwegian, "ID-en til advarselen du vil fjerne.").
					WithRequired(true)),
	)

// InfractionsListHandler handles the `/infractions list` command.
func InfractionsListHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("infractions", e)

	slog.Info("interaction `/infractions list` called.")
	data := e.SlashCommandInteractionData()
	user, hasUser := data.OptUser("user")

	if !hasUser {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"You must specify a user.",
			),
		)
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

	return e.CreateMessage(message)
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

	err := model.DeleteInfractionBySqid(infID, guild.ID)
	if err != nil {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"Failed to delete infraction.",
			),
		)
	}

	return e.CreateMessage(
		interactions.EphemeralMessageContent(
			"Infraction deleted.",
		),
	)
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

	user, err := e.Client().Rest.GetUser(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if e.User().ID != parentIx.User.ID {
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"You can only paginate responses from your own commands.",
			),
		)
	}

	mcb, mub, err := getUserInfractionsAndUpdateMessage(false, offset, &guild, user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}
	if mcb != nil {
		return e.CreateMessage(*mcb)
	} else if mub != nil {
		return e.UpdateMessage(*mub)
	}
	return nil
}

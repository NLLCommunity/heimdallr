package infractions

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

// UserInfractionsCommand lets users view their own infractions.
var UserInfractionsCommand = discord.SlashCommandCreate{
	Name: "warnings",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "advarsler",
	},
	Description: "View your warnings.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Se advarslene dine.",
	},

	Contexts:         []discord.InteractionContextType{discord.InteractionContextTypeGuild},
	IntegrationTypes: []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
}

func UserInfractionsHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("infractions", e)

	user := e.User()
	guild, ok := e.Guild()
	if !ok {
		slog.Warn("No guild id found in event.", "guild", guild)
		return interactions.ErrEventNoGuildID
	}

	message, err := getUserInfractionsAndMakeMessage(false, &guild, &user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}

	return e.CreateMessage(message.Build())
}

func UserInfractionButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("infractions", e)
	offsetStr := e.Vars["offset"]
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return fmt.Errorf("failed to parse offset: %w", err)
	}

	user := e.User()
	guild, ok := e.Guild()
	if !ok {
		return interactions.ErrEventNoGuildID
	}

	mcb, mub, err := getUserInfractionsAndUpdateMessage(false, offset, &guild, &user)
	if err != nil {
		slog.Error("Error occurred getting infractions", "err", err)
	}
	if mcb != nil {
		return e.CreateMessage(mcb.Build())
	}
	return e.UpdateMessage(mub.Build())
}

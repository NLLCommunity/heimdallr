package modmail

import (
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

func TestRegisterInstallsModmailRoutes(t *testing.T) {
	router := handler.New()
	Interactions(router)

	require.True(t, router.Match("/modmail/report-button/1/2/3/4", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	require.True(t, router.Match("/modmail/report-modal/1/2/3/4", discord.InteractionTypeModalSubmit, 0))
	require.True(t, router.Match("/modmail/report-message/1/2", discord.InteractionTypeModalSubmit, 0))
}

func TestReportModalsContainOnlyTheirLabels(t *testing.T) {
	messageModal := reportMessageModal("/modmail/report-message/1/2")
	require.Len(t, messageModal.Components, 1)
	require.Equal(t, discord.ComponentTypeLabel, messageModal.Components[0].Type())

	modal := reportModal("/modmail/report-modal/1/2/3/4")
	require.Len(t, modal.Components, 2)
	require.Equal(t, discord.ComponentTypeLabel, modal.Components[0].Type())
	require.Equal(t, discord.ComponentTypeLabel, modal.Components[1].Type())
}

func TestCreateButtonLabelRequiresAtLeastThreeCharacters(t *testing.T) {
	command := ModmailAdmin.Build().(discord.SlashCommandCreate)

	var createButton discord.ApplicationCommandOptionSubCommand
	foundCreateButton := false
	for _, option := range command.Options {
		subcommand, ok := option.(discord.ApplicationCommandOptionSubCommand)
		if ok && subcommand.Name == "create-button" {
			createButton = subcommand
			foundCreateButton = true
			break
		}
	}
	require.True(t, foundCreateButton)

	var label discord.ApplicationCommandOptionString
	foundLabel := false
	for _, option := range createButton.Options {
		stringOption, ok := option.(discord.ApplicationCommandOptionString)
		if ok && stringOption.Name == "label" {
			label = stringOption
			foundLabel = true
			break
		}
	}
	require.True(t, foundLabel)
	require.NotNil(t, label.MinLength)
	require.Equal(t, 3, *label.MinLength)
}

func TestModmailRouteIDsRemainCompatible(t *testing.T) {
	id, err := modmailReportButtonRoute.CustomID(modmailReportVars{
		Role: snowflake.ID(1), Channel: snowflake.ID(2), MaxActive: 3, SlowMode: "4",
	})
	require.NoError(t, err)
	require.Equal(t, "/modmail/report-button/1/2/3/4", id)

	id, err = modmailReportModalRoute.CustomIDVars(mapReportVars("1", "2", "3", "4"))
	require.NoError(t, err)
	require.Equal(t, "/modmail/report-modal/1/2/3/4", id)

	id, err = modmailReportMessageRoute.CustomID(modmailReportMessageVars{
		ChannelID: snowflake.ID(1), MessageID: snowflake.ID(2),
	})
	require.NoError(t, err)
	require.Equal(t, "/modmail/report-message/1/2", id)
}

func TestModmailReportButtonRouteRoundsFractionalSlowMode(t *testing.T) {
	slowMode := 1500 * time.Millisecond

	id, err := modmailReportButtonRoute.CustomID(modmailReportVars{
		Role: snowflake.ID(1), Channel: snowflake.ID(2), MaxActive: 3,
		SlowMode: slowModeCustomIDValue(slowMode),
	})
	require.NoError(t, err)
	require.Equal(t, "/modmail/report-button/1/2/3/2", id)
}

func mapReportVars(role, channel, maxActive, slowMode string) rave.Vars {
	return rave.Vars{
		"role": role, "channel": channel, "max-active": maxActive, "slow-mode": slowMode,
	}
}

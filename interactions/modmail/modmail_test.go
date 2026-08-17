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

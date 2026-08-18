package admin

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterInstallsAdminComponentAndModalRoutes(t *testing.T) {
	router := handler.New()
	Interactions(router)

	componentPaths := []string{
		"/admin/show-all-button",
		"/admin/gatekeep-message/button",
		"/admin/join-message/button",
		"/admin/leave-message/button",
		"/admin/ban-footer/button",
	}
	for _, path := range componentPaths {
		require.True(t, router.Match(path, discord.InteractionTypeComponent, int(discord.ComponentTypeButton)), path)
	}

	modalPaths := []string{
		"/admin/gatekeep-message/modal",
		"/admin/join-message/modal",
		"/admin/leave-message/modal",
		"/admin/ban-footer/modal",
	}
	for _, path := range modalPaths {
		require.True(t, router.Match(path, discord.InteractionTypeModalSubmit, 0), path)
	}
}

func TestGatekeepMessageCommandExposesOnlyResetAndRetainsModalWorkflow(t *testing.T) {
	command := Admin.Build().(discord.SlashCommandCreate)

	var gatekeepMessage discord.ApplicationCommandOptionSubCommand
	found := false
	for _, option := range command.Options {
		subcommand, ok := option.(discord.ApplicationCommandOptionSubCommand)
		if ok && subcommand.Name == "gatekeep-message" {
			gatekeepMessage = subcommand
			found = true
			break
		}
	}
	require.True(t, found)
	require.Len(t, gatekeepMessage.Options, 1)
	require.Equal(t, "reset", gatekeepMessage.Options[0].OptionName())

	router := handler.New()
	Interactions(router)
	require.True(t, router.Match("/admin/gatekeep-message/button", discord.InteractionTypeComponent, int(discord.ComponentTypeButton)))
	require.True(t, router.Match("/admin/gatekeep-message/modal", discord.InteractionTypeModalSubmit, 0))
}

func TestSectionEmbed_StripsLeadingHeading(t *testing.T) {
	// Six of the seven *Info helpers begin with "## Title\n…". The
	// section heading must be removed so it isn't rendered twice — once
	// in the embed's title bar and once at the top of the description.
	got := sectionEmbed("Gatekeep", "## Gatekeep settings\n**Enabled:** yes\n> help text")
	assert.Equal(t, "Gatekeep", got.Title)
	assert.Equal(t, "**Enabled:** yes\n> help text", got.Description)
}

func TestSectionEmbed_NoLeadingHeading(t *testing.T) {
	// modChannelInfo and infractionInfo don't lead with "## ". Their
	// entire return value must reach the embed description intact.
	got := sectionEmbed("Moderator channel", "**Moderator channel:** <#42>\n> blurb")
	assert.Equal(t, "Moderator channel", got.Title)
	assert.Equal(t, "**Moderator channel:** <#42>\n> blurb", got.Description)
}

func TestSectionEmbed_TrimsSurroundingWhitespace(t *testing.T) {
	got := sectionEmbed("Title", "\n\n## Title\n  body line  \n\n")
	assert.Equal(t, "Title", got.Title)
	assert.Equal(t, "body line", got.Description)
}

func TestSectionEmbed_HeadingOnly(t *testing.T) {
	// A degenerate input that's just "## Title" with no body line —
	// title still wins, description goes empty rather than echoing
	// the heading.
	got := sectionEmbed("Title", "## Title")
	assert.Equal(t, "Title", got.Title)
	assert.Empty(t, got.Description)
}

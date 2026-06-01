package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

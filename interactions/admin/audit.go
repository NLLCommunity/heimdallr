package admin

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/audit"
)

// logSettingsCommandUpdate writes a settings.update audit log entry for
// changes made via slash command / button / modal flows. The event type
// is shared with the web dashboard's settings updates so they appear
// under the same Event filter; the entry's Source field distinguishes
// "command" from "web" for future filtering.
//
// Pass the section identifier matching the dashboard route (e.g.
// "anti_spam", "mod_channel") so the viewer's section-name resolver
// renders the same label regardless of where the change originated.
func logSettingsCommandUpdate(
	guildID snowflake.ID,
	user discord.User,
	section string,
	fields map[string]any,
) {
	details := map[string]any{
		"section":        section,
		"actor_username": user.Username,
	}
	for k, v := range fields {
		details[k] = v
	}
	uid := user.ID
	gid := guildID
	audit.Log(audit.Entry{
		GuildID:    guildID,
		EventType:  audit.EventSettingsUpdate,
		ActorID:    &uid,
		ActorKind:  audit.ActorUser,
		TargetID:   &gid,
		TargetKind: audit.TargetGuild,
		Source:     audit.SourceCommand,
		Details:    details,
	})
}

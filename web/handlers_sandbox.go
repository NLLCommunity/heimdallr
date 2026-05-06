package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

// maxSandboxBodyBytes caps the raw component JSON the sandbox accepts. Sized
// well below the global 1 MiB body limit because legitimate Discord component
// payloads are kilobytes at most; the gap exists to reject deeply nested
// adversarial input early.
const maxSandboxBodyBytes = 64 * 1024

func handleSandbox(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		guild, _ := client.Caches.Guild(guildID)
		nav := layouts.NavData{
			User:      session,
			GuildID:   guildIDStr,
			GuildName: guild.Name,
		}

		renderSafe(w, r, pages.Sandbox(nav, pages.SandboxData{
			GuildID:  guildIDStr,
			Channels: guildChannels(client, guildID),
		}))
	}
}

// handleSandboxSend posts an arbitrary V2-component message to a channel as
// the bot. Admin-gated, but we still apply a per-user rate limit (rather than
// per-IP) so that a single hostile or compromised admin can't burn through
// the bot's Discord quota by spamming sandbox sends.
func handleSandboxSend(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		session := sessionFromContext(r.Context())
		if session == nil {
			// authMiddleware should have caught this; treat as 401-equivalent.
			renderSafe(w, r, components.AlertError("Not signed in."))
			return
		}
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			renderSafe(w, r, components.AlertError("Rate limited. Please wait a moment before sending again."))
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		channelIDStr := r.FormValue("channel_id")
		componentsJSON := r.FormValue("components_json")

		if len(componentsJSON) > maxSandboxBodyBytes {
			renderSafe(w, r, components.AlertError("Components JSON too large."))
			return
		}
		if strings.TrimSpace(componentsJSON) == "" {
			renderSafe(w, r, components.AlertError("Invalid components JSON."))
			return
		}

		channelID, err := snowflake.Parse(channelIDStr)
		if err != nil {
			renderSafe(w, r, components.AlertError("Invalid channel."))
			return
		}

		ch, chOk := client.Caches.GuildMessageChannel(channelID)
		if !chOk || ch.GuildID() != guildID {
			renderSafe(w, r, components.AlertError("Channel not found in this guild."))
			return
		}

		emojiMap := utils.BuildEmojiMap(client, guildID)
		discordComponents, err := utils.BuildV2MessageNoTemplate(componentsJSON, emojiMap)
		if err != nil {
			slog.Error("failed to build sandbox components", "error", err)
			renderSafe(w, r, components.AlertError("Invalid components JSON."))
			return
		}

		_, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
			Flags:      discord.MessageFlagIsComponentsV2,
			Components: discordComponents,
		})
		if err != nil {
			slog.Error("failed to send Discord message", "error", err, "channel_id", channelID)
			renderSafe(w, r, components.AlertError("Failed to send message."))
			return
		}

		renderSafe(w, r, components.AlertSuccess("Message sent!"))
	}
}

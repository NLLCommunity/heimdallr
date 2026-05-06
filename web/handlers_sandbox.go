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
//
// Errors render as text/html AlertError partials with appropriate 4xx/5xx
// status codes. The HTMX swap config (htmx-config.js) treats 4xx/5xx as errors
// AND swaps when Content-Type is text/html, so the alert lands inline in
// #send-result while htmx:responseError suppresses its toast for HTML bodies.
// Status codes also let logs and intermediaries distinguish failures from the
// 200 success.
func handleSandboxSend(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderError := func(status int, message string) {
			renderSafeStatus(w, r, status, components.AlertError(message))
		}

		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		session := sessionFromContext(r.Context())
		if session == nil {
			// authMiddleware should have caught this; treat as 401-equivalent.
			renderError(http.StatusUnauthorized, "Not signed in.")
			return
		}
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			renderError(http.StatusTooManyRequests, "Rate limited. Please wait a moment before sending again.")
			return
		}

		if err := r.ParseForm(); err != nil {
			renderError(http.StatusBadRequest, "Invalid form data.")
			return
		}
		channelIDStr := r.FormValue("channel_id")
		componentsJSON := r.FormValue("components_json")

		if len(componentsJSON) > maxSandboxBodyBytes {
			renderError(http.StatusRequestEntityTooLarge, "Components JSON too large.")
			return
		}
		if strings.TrimSpace(componentsJSON) == "" {
			renderError(http.StatusBadRequest, "Invalid components JSON.")
			return
		}

		channelID, err := snowflake.Parse(channelIDStr)
		if err != nil {
			renderError(http.StatusBadRequest, "Invalid channel.")
			return
		}

		ch, chOk := client.Caches.GuildMessageChannel(channelID)
		if !chOk || ch.GuildID() != guildID {
			renderError(http.StatusBadRequest, "Channel not found in this guild.")
			return
		}

		emojiMap := utils.BuildEmojiMap(client, guildID)
		discordComponents, err := utils.BuildV2MessageNoTemplate(componentsJSON, emojiMap)
		if err != nil {
			slog.Error("failed to build sandbox components", "error", err)
			renderError(http.StatusBadRequest, "Invalid components JSON.")
			return
		}

		_, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
			Flags:      discord.MessageFlagIsComponentsV2,
			Components: discordComponents,
		})
		if err != nil {
			slog.Error("failed to send Discord message", "error", err, "channel_id", channelID)
			renderError(http.StatusBadGateway, "Failed to send message.")
			return
		}

		renderSafe(w, r, components.AlertSuccess("Message sent!"))
	}
}

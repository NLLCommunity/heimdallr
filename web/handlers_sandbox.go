package web

import (
	"encoding/json"
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

		pages.Sandbox(nav, pages.SandboxData{
			GuildID:  guildIDStr,
			Channels: guildChannels(client, guildID),
		}).Render(r.Context(), w)
	}
}

func handleSandboxSend(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		channelIDStr := r.FormValue("channel_id")
		componentsJSON := r.FormValue("components_json")

		channelID, err := snowflake.Parse(channelIDStr)
		if err != nil {
			components.AlertError("Invalid channel.").Render(r.Context(), w)
			return
		}

		ch, chOk := client.Caches.GuildMessageChannel(channelID)
		if !chOk || ch.GuildID() != guildID {
			components.AlertError("Channel not found in this guild.").Render(r.Context(), w)
			return
		}

		// Parse and resolve emojis.
		var parsed any
		if err := json.Unmarshal([]byte(componentsJSON), &parsed); err != nil {
			components.AlertError("Invalid components JSON.").Render(r.Context(), w)
			return
		}

		emojiMap := buildEmojiMap(client, guildID)
		utils.ResolveEmojis(parsed, emojiMap)

		resolvedJSON, err := json.Marshal(parsed)
		if err != nil {
			components.AlertError("Failed to process components.").Render(r.Context(), w)
			return
		}

		discordComponents, err := utils.ParseComponents(string(resolvedJSON))
		if err != nil {
			slog.Error("failed to parse resolved components", "error", err)
			components.AlertError("Invalid components format.").Render(r.Context(), w)
			return
		}

		_, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
			Flags:      discord.MessageFlagIsComponentsV2,
			Components: discordComponents,
		})
		if err != nil {
			slog.Error("failed to send Discord message", "error", err, "channel_id", channelID)
			components.AlertError("Failed to send message.").Render(r.Context(), w)
			return
		}

		components.AlertSuccess("Message sent!").Render(r.Context(), w)
	}
}

// buildEmojiMap builds a lowercase emoji name → Emoji lookup from the guild cache.
func buildEmojiMap(client *bot.Client, guildID snowflake.ID) map[string]discord.Emoji {
	emojiMap := make(map[string]discord.Emoji)
	for emoji := range client.Caches.Emojis(guildID) {
		emojiMap[strings.ToLower(emoji.Name)] = emoji
	}
	return emojiMap
}

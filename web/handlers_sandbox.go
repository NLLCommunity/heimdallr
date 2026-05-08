package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

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

// maxSandboxContentChars caps the V1 message content the sandbox accepts when
// editing legacy messages, matching Discord's 2000-character ceiling. Counted
// in runes (not bytes) so multi-byte content isn't falsely rejected and so
// large all-ASCII payloads aren't silently waved through to a Discord 4xx.
const maxSandboxContentChars = 2000

// sandboxMessageLinkRegex matches Discord message URLs across the canary,
// ptb, and stable subdomains. The named groups carry the snowflake IDs.
var sandboxMessageLinkRegex = regexp.MustCompile(
	`^https://(?:canary\.|ptb\.)?discord\.com/channels/(?P<guild>\d+)/(?P<channel>\d+)/(?P<message>\d+)/?$`,
)

// errInvalidMessageLink is returned when a sandbox load request's link can't
// be parsed; surfaced to the admin verbatim.
var errInvalidMessageLink = errors.New("invalid message link")

// parseSandboxMessageLink returns guild/channel/message snowflakes from a
// Discord message link, or an error if the link doesn't match the expected
// shape.
func parseSandboxMessageLink(link string) (guildID, channelID, messageID snowflake.ID, err error) {
	m := sandboxMessageLinkRegex.FindStringSubmatch(strings.TrimSpace(link))
	if m == nil {
		return 0, 0, 0, errInvalidMessageLink
	}
	if guildID, err = snowflake.Parse(m[1]); err != nil {
		return 0, 0, 0, err
	}
	if channelID, err = snowflake.Parse(m[2]); err != nil {
		return 0, 0, 0, err
	}
	if messageID, err = snowflake.Parse(m[3]); err != nil {
		return 0, 0, 0, err
	}
	return guildID, channelID, messageID, nil
}

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
			IsAdmin:   true,
			IsPostMod: true,
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

		// checkGuildAdmin already enforced session presence; sessionFromContext
		// is therefore non-nil here (authMiddleware injects the session before
		// any /sandbox/* handler runs).
		session := sessionFromContext(r.Context())
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

		// Empty AllowedMentions suppresses @everyone / role / user pings that
		// would otherwise fire from text_display markdown — same default as
		// every other bot-authored message in the codebase (interactions/*,
		// listeners/*, and the post-publish path in web/posts).
		_, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
			Flags:           discord.MessageFlagIsComponentsV2,
			Components:      discordComponents,
			AllowedMentions: &discord.AllowedMentions{},
		})
		if err != nil {
			slog.Error("failed to send Discord message", "error", err, "channel_id", channelID)
			renderError(http.StatusBadGateway, "Failed to send message.")
			return
		}

		renderSafe(w, r, components.AlertSuccess("Message sent!"))
	}
}

// sandboxLoadResponse is the JSON shape returned by /sandbox/load. The editor
// reads `is_v2` to choose between the V2 component editor (Components) and a
// plain content textarea (Content).
type sandboxLoadResponse struct {
	ChannelID  string            `json:"channel_id"`
	MessageID  string            `json:"message_id"`
	IsV2       bool              `json:"is_v2"`
	Content    string            `json:"content"`
	Components []json.RawMessage `json:"components"`
}

// handleSandboxLoad fetches a Discord message by link and returns its content
// or component tree as JSON, ready for the editor to populate. Bot-authored
// messages only — Discord won't let us edit anyone else's, so loading them
// would be a dead end. Reuses the sandbox rate limiter because each call hits
// Discord's REST API.
func handleSandboxLoad(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONError := func(status int, message string) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		}

		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		// session is non-nil after checkGuildAdmin succeeds — see handleSandboxSend.
		session := sessionFromContext(r.Context())
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			writeJSONError(http.StatusTooManyRequests, "Rate limited. Please wait a moment before trying again.")
			return
		}

		if err := r.ParseForm(); err != nil {
			writeJSONError(http.StatusBadRequest, "Invalid form data.")
			return
		}

		linkGuildID, channelID, messageID, err := parseSandboxMessageLink(r.FormValue("link"))
		if err != nil {
			writeJSONError(http.StatusBadRequest, "Invalid message link.")
			return
		}
		if linkGuildID != guildID {
			writeJSONError(http.StatusBadRequest, "Message link is not in this server.")
			return
		}

		ch, chOk := client.Caches.GuildMessageChannel(channelID)
		if !chOk || ch.GuildID() != guildID {
			writeJSONError(http.StatusBadRequest, "Channel not found in this guild.")
			return
		}

		message, err := client.Rest.GetMessage(channelID, messageID)
		if err != nil {
			slog.Error("failed to fetch Discord message", "error", err, "channel_id", channelID, "message_id", messageID)
			writeJSONError(http.StatusBadGateway, "Failed to fetch message.")
			return
		}

		if message.Author.ID != client.ID() {
			writeJSONError(http.StatusBadRequest, "Can only edit messages sent by this bot.")
			return
		}

		isV2 := message.Flags.Has(discord.MessageFlagIsComponentsV2)

		// Marshal components individually so the JS editor receives the same
		// shape as it serializes back — Discord-typed objects keyed by numeric
		// `type` codes. json.RawMessage avoids a second decode/encode pass.
		raws := make([]json.RawMessage, 0, len(message.Components))
		for _, c := range message.Components {
			b, err := json.Marshal(c)
			if err != nil {
				slog.Error("failed to marshal component", "error", err)
				writeJSONError(http.StatusInternalServerError, "Failed to serialize message components.")
				return
			}
			raws = append(raws, b)
		}

		resp := sandboxLoadResponse{
			ChannelID:  channelID.String(),
			MessageID:  messageID.String(),
			IsV2:       isV2,
			Content:    message.Content,
			Components: raws,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode sandbox load response", "error", err)
		}
	}
}

// handleSandboxEdit edits an already-sent bot message. V2 path uses
// Components+IsComponentsV2; V1 path edits Content only. Mirrors the security
// posture of /sandbox/send: admin-only, per-user rate limited, channel must
// belong to the guild, errors render as text/html AlertError partials.
func handleSandboxEdit(client *bot.Client, limiter *keyedRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderError := func(status int, message string) {
			renderSafeStatus(w, r, status, components.AlertError(message))
		}

		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		// session is non-nil after checkGuildAdmin succeeds — see handleSandboxSend.
		session := sessionFromContext(r.Context())
		if !limiter.getLimiter(session.UserID.String()).Allow() {
			renderError(http.StatusTooManyRequests, "Rate limited. Please wait a moment before sending again.")
			return
		}

		if err := r.ParseForm(); err != nil {
			renderError(http.StatusBadRequest, "Invalid form data.")
			return
		}

		channelID, err := snowflake.Parse(r.FormValue("channel_id"))
		if err != nil {
			renderError(http.StatusBadRequest, "Invalid channel.")
			return
		}
		messageID, err := snowflake.Parse(r.FormValue("message_id"))
		if err != nil {
			renderError(http.StatusBadRequest, "Invalid message.")
			return
		}

		ch, chOk := client.Caches.GuildMessageChannel(channelID)
		if !chOk || ch.GuildID() != guildID {
			renderError(http.StatusBadRequest, "Channel not found in this guild.")
			return
		}

		isV2 := r.FormValue("is_v2") == "true"

		var update discord.MessageUpdate
		if isV2 {
			componentsJSON := r.FormValue("components_json")
			if len(componentsJSON) > maxSandboxBodyBytes {
				renderError(http.StatusRequestEntityTooLarge, "Components JSON too large.")
				return
			}
			if strings.TrimSpace(componentsJSON) == "" {
				renderError(http.StatusBadRequest, "Invalid components JSON.")
				return
			}

			emojiMap := utils.BuildEmojiMap(client, guildID)
			discordComponents, err := utils.BuildV2MessageNoTemplate(componentsJSON, emojiMap)
			if err != nil {
				slog.Error("failed to build sandbox components", "error", err)
				renderError(http.StatusBadRequest, "Invalid components JSON.")
				return
			}
			// Edits replace AllowedMentions; without setting it explicitly,
			// Discord re-evaluates mentions from the new content and pings
			// anyone @-mentioned in the edited markdown.
			update = discord.NewMessageUpdateV2(discordComponents).WithAllowedMentions(&discord.AllowedMentions{})
		} else {
			content := r.FormValue("content")
			if utf8.RuneCountInString(content) > maxSandboxContentChars {
				renderError(http.StatusRequestEntityTooLarge, "Message content too large.")
				return
			}
			update = discord.NewMessageUpdate().WithContent(content).WithAllowedMentions(&discord.AllowedMentions{})
		}

		if _, err := client.Rest.UpdateMessage(channelID, messageID, update); err != nil {
			slog.Error("failed to edit Discord message", "error", err, "channel_id", channelID, "message_id", messageID)
			renderError(http.StatusBadGateway, "Failed to edit message.")
			return
		}

		renderSafe(w, r, components.AlertSuccess("Message updated!"))
	}
}

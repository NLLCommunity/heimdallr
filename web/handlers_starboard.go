package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"

	"github.com/NLLCommunity/heimdallr/model"
	starboardsvc "github.com/NLLCommunity/heimdallr/starboard"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
)

const (
	starboardDefaultThreshold = 4
	starboardMaxThreshold     = 100000
)

func handleStarboards(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}
		renderStarboardsPage(w, r, client, guildID, starboardNotice(r.URL.Query().Get("saved")), "", http.StatusOK)
	}
}

func handleStarboardCreate(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildID, ok := checkGuildAdmin(w, r, client, r.PathValue("id"))
		if !ok {
			return
		}
		if !requireStarboardService(w) {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		board, err := parseStarboardBoardForm(r.Form, guildID, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !dashboardChannelBelongsToGuild(client, guildID, board.ChannelID) {
			http.Error(w, "destination channel does not belong to this guild", http.StatusBadRequest)
			return
		}
		if err := starboardsvc.Default.SaveBoard(&board); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		redirectStarboards(w, r, guildID, "created")
	}
}

func handleStarboardSave(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildID, ok := checkGuildAdmin(w, r, client, r.PathValue("id"))
		if !ok {
			return
		}
		if !requireStarboardService(w) {
			return
		}
		boardID, err := parseStarboardID(r.PathValue("boardID"))
		if err != nil {
			http.Error(w, "invalid board ID", http.StatusBadRequest)
			return
		}
		existing, err := findStarboard(guildID, boardID)
		if err != nil {
			writeStarboardLookupError(w, err)
			return
		}
		if err = r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		board, err := parseStarboardBoardForm(r.Form, guildID, boardID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		missingUnchangedDestinationOnDisable := !board.Enabled && existing.ChannelID == board.ChannelID
		if !dashboardChannelBelongsToGuild(client, guildID, board.ChannelID) && !missingUnchangedDestinationOnDisable {
			http.Error(w, "destination channel does not belong to this guild", http.StatusBadRequest)
			return
		}
		if err = starboardsvc.Default.SaveBoard(&board); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		redirectStarboards(w, r, guildID, "updated")
	}
}

func handleStarboardDelete(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildID, ok := checkGuildAdmin(w, r, client, r.PathValue("id"))
		if !ok {
			return
		}
		if !requireStarboardService(w) {
			return
		}
		boardID, err := parseStarboardID(r.PathValue("boardID"))
		if err != nil {
			http.Error(w, "invalid board ID", http.StatusBadRequest)
			return
		}
		if _, err = findStarboard(guildID, boardID); err != nil {
			writeStarboardLookupError(w, err)
			return
		}
		if err = starboardsvc.Default.DeleteBoard(guildID, boardID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		redirectStarboards(w, r, guildID, "deleted")
	}
}

func handleStarboardMaster(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildID, ok := checkGuildAdmin(w, r, client, r.PathValue("id"))
		if !ok {
			return
		}
		if !requireStarboardService(w) {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		enabled, err := parseStarboardBool(r.FormValue("enabled"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err = starboardsvc.Default.SetEnabled(guildID, enabled); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		redirectStarboards(w, r, guildID, "master")
	}
}

func parseStarboardBoardForm(values url.Values, guildID snowflake.ID, boardID uint64) (model.Starboard, error) {
	name := strings.TrimSpace(values.Get("name"))
	if name == "" {
		return model.Starboard{}, errors.New("board name is required")
	}
	channelID, err := snowflake.Parse(strings.TrimSpace(values.Get("channel")))
	if err != nil || channelID == 0 {
		return model.Starboard{}, errors.New("select a valid destination channel")
	}
	emoji, err := starboardsvc.ParseEmoji(values.Get("emoji"))
	if err != nil {
		return model.Starboard{}, fmt.Errorf("invalid positive emoji: %w", err)
	}
	thresholdText := strings.TrimSpace(values.Get("threshold"))
	if thresholdText == "" {
		thresholdText = strconv.Itoa(starboardDefaultThreshold)
	}
	threshold, err := strconv.Atoi(thresholdText)
	if err != nil || threshold < 1 || threshold > starboardMaxThreshold {
		return model.Starboard{}, fmt.Errorf("threshold must be between 1 and %d", starboardMaxThreshold)
	}
	enabled, err := parseStarboardBool(values.Get("enabled"))
	if err != nil {
		return model.Starboard{}, err
	}
	return model.Starboard{
		ID: boardID, GuildID: guildID, Name: name, ChannelID: channelID,
		Emoji: emoji, Threshold: threshold, Enabled: enabled,
	}, nil
}

func parseStarboardBool(raw string) (bool, error) {
	switch raw {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, errors.New("enabled must be true or false")
	}
}

func parseStarboardID(raw string) (uint64, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("invalid board ID")
	}
	return id, nil
}

func findStarboard(guildID snowflake.ID, boardID uint64) (*model.Starboard, error) {
	var board model.Starboard
	if err := model.DB.Where("guild_id = ? AND id = ?", guildID, boardID).First(&board).Error; err != nil {
		return nil, err
	}
	return &board, nil
}

func dashboardChannelBelongsToGuild(client *bot.Client, guildID, channelID snowflake.ID) bool {
	channel, ok := client.Caches.Channel(channelID)
	return ok && channel.GuildID() == guildID
}

func starboardDestinationChannels(groups []components.ChannelGroup) []components.ChannelGroup {
	filtered := make([]components.ChannelGroup, 0, len(groups))
	for _, group := range groups {
		channels := make([]components.ChannelInfo, 0, len(group.Channels))
		for _, channel := range group.Channels {
			if channel.Type == discord.ChannelTypeGuildText {
				channels = append(channels, channel)
			}
		}
		if len(channels) != 0 {
			filtered = append(filtered, components.ChannelGroup{Name: group.Name, Channels: channels})
		}
	}
	return filtered
}

func renderStarboardsPage(w http.ResponseWriter, r *http.Request, client *bot.Client, guildID snowflake.ID, notice, pageError string, status int) {
	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		http.Error(w, "failed to load starboard settings", http.StatusInternalServerError)
		return
	}
	var boards []model.Starboard
	if err = model.DB.Where("guild_id = ?", guildID).Order("id").Find(&boards).Error; err != nil {
		http.Error(w, "failed to load starboards", http.StatusInternalServerError)
		return
	}
	guild, _ := client.Caches.Guild(guildID)
	nav := layouts.NavData{User: sessionFromContext(r.Context()), GuildID: guildID.String(), GuildName: guild.Name, IsAdmin: true, IsPostMod: true}
	renderSafeStatus(w, r, status, pages.Starboards(nav, pages.StarboardsData{
		GuildID: guildID.String(), MasterEnabled: settings.StarboardEnabled,
		Boards: boards, Channels: starboardDestinationChannels(guildChannels(client, guildID)), Notice: notice, Error: pageError,
	}))
}

func requireStarboardService(w http.ResponseWriter) bool {
	if starboardsvc.Default != nil {
		return true
	}
	http.Error(w, "starboard service is unavailable", http.StatusServiceUnavailable)
	return false
}

func writeStarboardLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "starboard not found", http.StatusNotFound)
		return
	}
	http.Error(w, "failed to load starboard", http.StatusInternalServerError)
}

func redirectStarboards(w http.ResponseWriter, r *http.Request, guildID snowflake.ID, saved string) {
	http.Redirect(w, r, "/guild/"+guildID.String()+"/starboards?saved="+url.QueryEscape(saved), http.StatusSeeOther)
}

func starboardNotice(saved string) string {
	switch saved {
	case "created":
		return "Board created."
	case "updated":
		return "Board settings saved."
	case "deleted":
		return "Board deletion queued; its copies will be removed in the background."
	case "master":
		return "Server starboard setting saved."
	default:
		return ""
	}
}

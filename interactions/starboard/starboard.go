package starboard

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	starboardsvc "github.com/NLLCommunity/heimdallr/starboard"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

func messageOptions(boardRequired bool) []rave.CommandOption {
	board := rave.OptionInt("board", "Board ID shown in the dashboard").WithMinValue(1).WithRequired(boardRequired)
	message := rave.OptionString("message", "Discord message link or message ID").WithRequired(true)
	channel := rave.OptionChannel("channel", "Channel containing the message ID")
	if !boardRequired {
		return []rave.CommandOption{message, board, channel}
	}
	return []rave.CommandOption{
		board,
		message,
		channel,
	}
}

func channelOptions() []rave.CommandOption {
	return []rave.CommandOption{
		rave.OptionChannel("channel", "Source channel or thread").WithRequired(true),
		rave.OptionInt("board", "Board ID, or omit for the whole server").WithMinValue(1),
	}
}

var Command = rave.Slash("starboard", "Moderate starboard posts and exclusions").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionManageMessages).
	AddOptions(
		rave.SubCommand("remove", "Remove and suppress a message from one board").AddOptions(messageOptions(true)...).Handle(moderationHandler("remove")),
		rave.SubCommand("restore", "Clear suppression and reevaluate a message").AddOptions(messageOptions(true)...).Handle(moderationHandler("restore")),
		rave.SubCommand("blacklist-message", "Exclude a message from one board or the whole server").AddOptions(messageOptions(false)...).Handle(moderationHandler("blacklist-message")),
		rave.SubCommand("unblacklist-message", "Remove a message exclusion").AddOptions(messageOptions(false)...).Handle(moderationHandler("unblacklist-message")),
		rave.SubCommand("blacklist-channel", "Exclude a channel from one board or the whole server").AddOptions(channelOptions()...).Handle(moderationHandler("blacklist-channel")),
		rave.SubCommand("unblacklist-channel", "Remove a channel exclusion").AddOptions(channelOptions()...).Handle(moderationHandler("unblacklist-channel")),
	)

var Interactions = rave.Bundle(Command)

func moderationHandler(action string) handler.CommandHandler {
	return func(e *handler.CommandEvent) error {
		if e.GuildID() == nil || e.Member() == nil {
			return interactions.ErrEventNoGuildID
		}
		if err := e.DeferCreateMessage(true); err != nil {
			return err
		}
		guildID := *e.GuildID()
		member := e.Member().Member
		data := e.SlashCommandInteractionData()
		boardID, err := moderationBoardID(data, action)
		if err != nil {
			return moderationInputError(e, err)
		}

		var board *model.Starboard
		if boardID != 0 {
			board, err = findModerationBoard(guildID, boardID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return moderationInputError(e, errors.New("that board does not exist in this guild"))
			}
			if err != nil {
				return moderationFailure(e, err)
			}
			if board.Deleting {
				return moderationInputError(e, errors.New("that board is being deleted"))
			}
		}

		if !authorizedForModeration(e.Client(), member, board) {
			return moderationReply(e, moderationPermissionMessage(boardID))
		}

		channelID, messageID, err := moderationTarget(e.Client(), data, guildID, action)
		if err != nil {
			return moderationInputError(e, err)
		}
		if starboardsvc.Default == nil {
			return moderationFailure(e, errors.New("starboard service is unavailable"))
		}
		if err = starboardsvc.Default.Moderate(guildID, boardID, action, channelID, messageID); err != nil {
			return moderationFailure(e, err)
		}
		return moderationReply(e, moderationSuccessMessage(action))
	}
}

func moderationBoardID(data discord.SlashCommandInteractionData, action string) (uint64, error) {
	value, present := data.OptInt("board")
	if !present {
		if action == "remove" || action == "restore" {
			return 0, errors.New("select a board")
		}
		return 0, nil
	}
	if value < 1 {
		return 0, errors.New("board ID must be positive")
	}
	return uint64(value), nil
}

func moderationTarget(client *bot.Client, data discord.SlashCommandInteractionData, guildID snowflake.ID, action string) (snowflake.ID, snowflake.ID, error) {
	resolved, hasChannel := data.OptChannel("channel")
	channelID := snowflake.ID(0)
	if hasChannel {
		channelID = resolved.ID
		if _, ok := resolveGuildChannel(client, guildID, channelID); !ok {
			return 0, 0, errors.New("selected channel does not belong to this guild")
		}
	}
	if action == "blacklist-channel" || action == "unblacklist-channel" {
		if !hasChannel {
			return 0, 0, errors.New("select a channel")
		}
		return channelID, 0, nil
	}

	raw, ok := data.OptString("message")
	if !ok {
		return 0, 0, errors.New("provide a Discord message link or message ID")
	}
	channelID, messageID, err := parseMessageReference(raw, channelID, guildID)
	if err != nil {
		return 0, 0, err
	}
	if _, ok = resolveGuildChannel(client, guildID, channelID); !ok {
		return 0, 0, errors.New("message channel does not belong to this guild")
	}
	return channelID, messageID, nil
}

func findModerationBoard(guildID snowflake.ID, boardID uint64) (*model.Starboard, error) {
	var board model.Starboard
	if err := model.DB.Where("guild_id = ? AND id = ?", guildID, boardID).First(&board).Error; err != nil {
		return nil, err
	}
	return &board, nil
}

func authorizedForModeration(client *bot.Client, member discord.Member, board *model.Starboard) bool {
	if board == nil {
		return canModerateGuild(client.Caches, member)
	}
	if canModerateBoard(client.Caches, member, *board) {
		return true
	}
	channel, ok := resolveGuildChannel(client, board.GuildID, board.ChannelID)
	return ok && client.Caches.MemberPermissionsInChannel(channel, member).Has(discord.PermissionManageMessages)
}

func resolveGuildChannel(client *bot.Client, guildID, channelID snowflake.ID) (discord.GuildChannel, bool) {
	if channel, ok := cachedGuildChannel(client.Caches, guildID, channelID); ok {
		return channel, true
	}
	if client.Rest == nil {
		return nil, false
	}
	channel, err := client.Rest.GetChannel(channelID)
	if err != nil {
		return nil, false
	}
	guildChannel, ok := channel.(discord.GuildChannel)
	return guildChannel, ok && guildChannel.GuildID() == guildID
}

func moderationInputError(e *handler.CommandEvent, err error) error {
	return moderationReply(e, err.Error())
}

func moderationFailure(e *handler.CommandEvent, err error) error {
	slog.Error("starboard moderation failed", "error", err)
	responseErr := moderationReply(e, "Unable to update the starboard. Check the message, board, and permissions, then try again.")
	return errors.Join(err, responseErr)
}

func moderationReply(e *handler.CommandEvent, content string) error {
	update := discord.NewMessageUpdate().WithContent(content)
	update.AllowedMentions = &discord.AllowedMentions{}
	_, err := e.UpdateInteractionResponse(update)
	return err
}

func moderationPermissionMessage(boardID uint64) string {
	if boardID == 0 {
		return "You need the server-level Manage Messages permission or Administrator to change a server-wide blacklist."
	}
	return "You need Manage Messages in this board's destination channel or Administrator to moderate it."
}

func moderationSuccessMessage(action string) string {
	switch action {
	case "remove":
		return "Message suppressed for this board and its removal queued."
	case "restore":
		return "Message restored and queued for reevaluation."
	case "blacklist-message":
		return "Message blacklisted and matching copies queued for removal."
	case "unblacklist-message":
		return "Message blacklist removed and tracked entries queued for reevaluation."
	case "blacklist-channel":
		return "Channel blacklisted and matching copies queued for removal."
	case "unblacklist-channel":
		return "Channel blacklist removed and tracked entries queued for reevaluation."
	default:
		return fmt.Sprintf("Starboard action %q completed.", action)
	}
}

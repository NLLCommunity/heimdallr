package starboard

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
)

var errInvalidMessageReference = errors.New("invalid message reference")

func parseMessageReference(raw string, channelID, guildID snowflake.ID) (snowflake.ID, snowflake.ID, error) {
	raw = strings.TrimSpace(raw)
	if messageID, err := snowflake.Parse(raw); err == nil && messageID != 0 {
		if channelID == 0 {
			return 0, 0, fmt.Errorf("%w: a channel is required when using a message ID", errInvalidMessageReference)
		}
		return channelID, messageID, nil
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !validDiscordHost(u.Hostname()) {
		return 0, 0, fmt.Errorf("%w: use a Discord message link or message ID", errInvalidMessageReference)
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 4 || parts[0] != "channels" {
		return 0, 0, fmt.Errorf("%w: use a Discord message link", errInvalidMessageReference)
	}
	linkGuild, errGuild := snowflake.Parse(parts[1])
	linkChannel, errChannel := snowflake.Parse(parts[2])
	messageID, errMessage := snowflake.Parse(parts[3])
	if errGuild != nil || errChannel != nil || errMessage != nil || linkGuild == 0 || linkChannel == 0 || messageID == 0 {
		return 0, 0, fmt.Errorf("%w: malformed Discord message link", errInvalidMessageReference)
	}
	if linkGuild != guildID {
		return 0, 0, fmt.Errorf("%w: message link belongs to a different guild", errInvalidMessageReference)
	}
	if channelID != 0 && channelID != linkChannel {
		return 0, 0, fmt.Errorf("%w: selected channel does not match the message link", errInvalidMessageReference)
	}
	return linkChannel, messageID, nil
}

func validDiscordHost(host string) bool {
	switch strings.ToLower(host) {
	case "discord.com", "canary.discord.com", "ptb.discord.com":
		return true
	default:
		return false
	}
}

func canModerateGuild(caches cache.Caches, member discord.Member) bool {
	return caches.MemberPermissions(member).Has(discord.PermissionManageMessages)
}

func canModerateBoard(caches cache.Caches, member discord.Member, board model.Starboard) bool {
	if caches.MemberPermissions(member).Has(discord.PermissionAdministrator) {
		return true
	}
	channel, ok := cachedGuildChannel(caches, board.GuildID, board.ChannelID)
	if !ok {
		return false
	}
	return caches.MemberPermissionsInChannel(channel, member).Has(discord.PermissionManageMessages)
}

func cachedGuildChannel(caches cache.Caches, guildID, channelID snowflake.ID) (discord.GuildChannel, bool) {
	channel, ok := caches.Channel(channelID)
	if !ok || channel.GuildID() != guildID {
		return nil, false
	}
	return channel, true
}

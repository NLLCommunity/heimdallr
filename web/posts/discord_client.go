package posts

import (
	"encoding/json"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/utils"
)

// liveDiscord adapts disgo's REST client + a guild's emoji map to the
// DiscordClient interface used by the sync engine. The emoji map is captured
// at construction time; callers should rebuild the adapter per-publish so
// recently-added emojis are picked up.
type liveDiscord struct {
	client   *bot.Client
	emojiMap map[string]discord.Emoji
}

// NewLiveDiscord constructs an adapter for the given guild.
func NewLiveDiscord(client *bot.Client, guildID snowflake.ID) DiscordClient {
	return &liveDiscord{
		client:   client,
		emojiMap: utils.BuildEmojiMap(client, guildID),
	}
}

func (d *liveDiscord) chunkToComponents(chunk []any) ([]discord.LayoutComponent, error) {
	b, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	return utils.BuildV2MessageNoTemplate(string(b), d.emojiMap)
}

func (d *liveDiscord) SendV2(channelID snowflake.ID, chunk []any) (snowflake.ID, error) {
	comps, err := d.chunkToComponents(chunk)
	if err != nil {
		return 0, err
	}
	msg, err := d.client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: comps,
	})
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

func (d *liveDiscord) EditV2(channelID, messageID snowflake.ID, chunk []any) error {
	comps, err := d.chunkToComponents(chunk)
	if err != nil {
		return err
	}
	_, err = d.client.Rest.UpdateMessage(channelID, messageID, discord.NewMessageUpdateV2(comps))
	return err
}

func (d *liveDiscord) Delete(channelID, messageID snowflake.ID) error {
	return d.client.Rest.DeleteMessage(channelID, messageID)
}

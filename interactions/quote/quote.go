package quote

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/quote", QuoteHandler)

	return []discord.ApplicationCommandCreate{QuoteCommand}
}

var quoteUrlRegex = regexp.MustCompile(
	`https://discord.com/channels/(?P<guild>\d+)/(?P<channel>\d+)/(?P<message>\d+)`,
)

var QuoteCommand = discord.SlashCommandCreate{
	Name: "quote",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "siter",
	},
	Description: "Quote a message from a channel, using a message link.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Lag et sitat av ei melding.",
	},

	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionSendMessages),

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionString{
			Name: "link",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "lenke",
			},
			Description: "Link to the message you want to quote.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Lenke til meldinga du vil sitere.",
			},
			Required: true,
		},
		discord.ApplicationCommandOptionBool{
			Name: "show-reply-to",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "vis-svar-til",
			},
			Description: "Whether to show and link the message that the quoted message replies to. (Default: true)",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Om du vil vise og lenke til meldingen som sitatet er et svar til. (Standard er å vise)",
			},
			Required: false,
		},
	},
}

func QuoteHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("quote", e)

	var guildID snowflake.ID
	if e.GuildID() != nil {
		guildID = *e.GuildID()
	} else {
		return interactions.ErrEventNoGuildID
	}
	url := e.SlashCommandInteractionData().String("link")
	showReplyTo, isSet := e.SlashCommandInteractionData().OptBool("show-reply-to")
	if !isSet {
		showReplyTo = true
	}

	parts, err := parseMessageLink(url)
	if err != nil {
		_ = e.CreateMessage(interactions.EphemeralMessageContent("Invalid message link.").
			Build())
		return err
	}

	if parts.GuildId != guildID {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"Message link is not in this server.").Build())
	}

	message, err := e.Client().Rest().GetMessage(parts.ChannelId, parts.MessageId)
	if err != nil {
		_ = e.CreateMessage(interactions.EphemeralMessageContent(
			"Failed to fetch message.").Build())
		return err
	}

	channelName := "unknown channel"
	channel, err := e.Client().Rest().GetChannel(parts.ChannelId)
	if err == nil {
		prefix := getChannelTypePrefix(channel)
		channelName = prefix + channel.Name()
	} else {
		slog.Warn(
			"Failed to fetch channel",
			"guild_id", parts.GuildId,
			"channel_id", parts.ChannelId,
		)
	}

	if canRead, _ := userCanReadChannelMessages(e.User().ID, message.ChannelID, e.Client()); !canRead {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"You don't have permission to read messages in that channel.").Build())
	}

	embed := discord.NewEmbedBuilder().
		SetDescription(message.Content).
		SetAuthor(message.Author.EffectiveName(), "", message.Author.EffectiveAvatarURL()).
		SetTimestamp(message.CreatedAt).
		SetFooter(channelName, "")

	if len(message.Attachments) == 1 {
		att := message.Attachments[0]
		embed.SetImage(att.URL)
	} else if len(message.Attachments) > 1 {
		var lines []string
		for _, att := range message.Attachments {
			lines = append(lines, fmt.Sprintf("- [%s](%s)", att.Filename, att.URL))
		}
		embed.AddField("Attachments", strings.Join(lines, "\n"), false)
	}
	if message.ReferencedMessage != nil && showReplyTo {
		ref := message.ReferencedMessage

		msg := fmt.Sprintf("message by %s", ref.Author.Mention())
		if ref.Content != "" {
			msg = fmt.Sprintf("%s:\n", ref.Author.Mention())
			if len(ref.Content) > 140 {
				msg += addQuoteToMessage(ref.Content[:137] + "...")
			} else {
				msg += addQuoteToMessage(ref.Content)
			}
		}
		msg += fmt.Sprintf("\n%s", ref.JumpURL())

		embed.AddField("Reply to", msg, false)
	}

	resp := discord.NewMessageCreateBuilder().SetEmbeds(embed.Build())
	resp.AddContainerComponents(discord.NewActionRow(discord.NewLinkButton("Jump to message", url)))
	resp.SetAllowedMentions(&discord.AllowedMentions{})

	return e.CreateMessage(resp.Build())
}

func getChannelTypePrefix(channel discord.Channel) string {
	switch channel.Type() {
	case discord.ChannelTypeGuildVoice:
		return "🔊"
	case discord.ChannelTypeGuildStageVoice:
		return "🗣️"
	case discord.ChannelTypeGuildNews:
		return "📢"
	case discord.ChannelTypeGuildForum:
		return "🗪"
	case discord.ChannelTypeGuildPublicThread,
		discord.ChannelTypeGuildPrivateThread:
		return "🧵"
	default:
		return "#"
	}
}

func userCanReadChannelMessages(userID, channelID snowflake.ID, client bot.Client) (bool, error) {
	channel, err := client.Rest().GetChannel(channelID)
	if err != nil {
		return false, fmt.Errorf("failed to get channel: %w", err)
	}

	var guildID snowflake.ID
	var permissionOverwrites discord.PermissionOverwrites

	switch c := channel.(type) {
	case discord.GuildMessageChannel:
		guildID = c.GuildID()
		permissionOverwrites = c.PermissionOverwrites()
	default:
		return false, nil
	}

	guild, err := client.Rest().GetGuild(guildID, false)
	if err != nil {
		return false, fmt.Errorf("failed to get guild: %w", err)
	}

	member, err := client.Rest().GetMember(guildID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get member: %w", err)
	}

	if guild.OwnerID == userID {
		return true, nil
	}

	roleMap := make(map[snowflake.ID]discord.Role)
	for _, role := range guild.Roles {
		roleMap[role.ID] = role
	}

	canViewChannelLevel := -1
	canViewChannel := roleMap[guildID].Permissions&discord.PermissionViewChannel == discord.PermissionViewChannel
	canReadMessageHistoryLevel := -1
	canReadMessageHistory := roleMap[guildID].Permissions&discord.PermissionReadMessageHistory == discord.PermissionReadMessageHistory

	for _, roleID := range member.RoleIDs {
		role := roleMap[roleID]
		canReadMessageHistory = canReadMessageHistory ||
			role.Permissions&discord.PermissionReadMessageHistory == discord.PermissionReadMessageHistory
		canViewChannel = canViewChannel ||
			role.Permissions&discord.PermissionViewChannel == discord.PermissionViewChannel
	}

	for _, overwrite := range permissionOverwrites {
		switch o := overwrite.(type) {
		case discord.MemberPermissionOverwrite:
			if o.UserID == userID {
				if o.Allow&discord.PermissionReadMessageHistory == discord.PermissionReadMessageHistory {
					canReadMessageHistoryLevel = math.MaxInt
					canReadMessageHistory = true
				} else if o.Deny&discord.PermissionReadMessageHistory == discord.PermissionReadMessageHistory {
					canReadMessageHistoryLevel = math.MaxInt
					canReadMessageHistory = false
				}

				if o.Allow&discord.PermissionViewChannel == discord.PermissionViewChannel {
					canViewChannelLevel = math.MaxInt
					canViewChannel = true
				} else if o.Deny&discord.PermissionViewChannel == discord.PermissionViewChannel {
					canViewChannelLevel = math.MaxInt
					canViewChannel = false
				}
			}
		case discord.RolePermissionOverwrite:
			role := roleMap[o.RoleID]
			fmt.Printf("Role: %s\n", role.Name)

			if !slices.Contains(member.RoleIDs, o.RoleID) && o.RoleID != guildID {
				fmt.Println("User doesn't have role")
				continue
			}

			if o.Allow&discord.PermissionReadMessageHistory == discord.PermissionReadMessageHistory &&
				role.Position > canReadMessageHistoryLevel {
				canReadMessageHistory = true
				canReadMessageHistoryLevel = role.Position
			} else if o.Deny&discord.PermissionReadMessageHistory == discord.PermissionReadMessageHistory &&
				role.Position > canReadMessageHistoryLevel {
				canReadMessageHistory = false
				canReadMessageHistoryLevel = role.Position
			}

			if o.Allow&discord.PermissionViewChannel == discord.PermissionViewChannel &&
				role.Position > canViewChannelLevel {
				canViewChannel = true
				canViewChannelLevel = role.Position
			} else if o.Deny&discord.PermissionViewChannel == discord.PermissionViewChannel &&
				role.Position > canViewChannelLevel {
				canViewChannel = false
				canViewChannelLevel = role.Position
			}
		}
	}

	return canReadMessageHistory && canViewChannel, nil
}

type linkParts struct {
	GuildId   snowflake.ID
	ChannelId snowflake.ID
	MessageId snowflake.ID
}

func parseMessageLink(url string) (parts linkParts, err error) {
	matches := quoteUrlRegex.FindStringSubmatch(url)
	if len(matches) != 4 {
		return linkParts{}, errors.New("invalid message link")
	}

	guildStr := matches[1]
	channelStr := matches[2]
	messageStr := matches[3]

	guildId, err := snowflake.Parse(guildStr)
	if err != nil {
		return
	}
	channelId, err := snowflake.Parse(channelStr)
	if err != nil {
		return
	}
	messageId, err := snowflake.Parse(messageStr)
	if err != nil {
		return
	}

	parts = linkParts{
		GuildId:   guildId,
		ChannelId: channelId,
		MessageId: messageId,
	}
	return
}

func addQuoteToMessage(text string) string {
	origs := strings.Split(text, "\n")
	temp := make([]string, len(origs))
	for i, s := range origs {
		temp[i] = fmt.Sprintf("> %s", s)
	}
	return strings.Join(temp, "\n")
}

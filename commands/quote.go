package commands

import (
	"errors"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/myrkvi/heimdallr/utils"
	"math"
	"regexp"
	"slices"
	"strings"
)

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
	},
}

func QuoteHandler(e *handler.CommandEvent) error {
	url := e.SlashCommandInteractionData().String("link")
	parts, err := parseMessageLink(url)
	if err != nil {
		_ = respondWithContentEph(e, "Invalid message link.")
		return err
	}

	message, err := e.Client().Rest().GetMessage(parts.ChannelId, parts.MessageId)
	if err != nil {
		_ = respondWithContentEph(e, "Failed to fetch message.")
		return err
	}

	if canRead, _ := userCanReadChannelMessages(e.User().ID, message.ChannelID, e.Client()); !canRead {
		_ = respondWithContentEph(e, "You don't have permission to read messages in that channel.")
		return nil
	}

	embed := discord.NewEmbedBuilder().
		SetDescription(message.Content).
		SetAuthor(message.Author.EffectiveName(), "", message.Author.EffectiveAvatarURL()).
		SetTimestamp(message.CreatedAt)

	if len(message.Attachments) == 1 {
		att := message.Attachments[0]
		embed.SetImage(att.URL)
	} else if len(message.Attachments) > 1 {
		var lines []string
		for _, att := range message.Attachments {
			lines = append(lines, fmt.Sprintf("· [%s](%s)", att.Filename, att.URL))
		}
		embed.AddField("Attachments", "• "+strings.Join(lines, "\n"), false)
	}

	resp := discord.NewMessageCreateBuilder().SetEmbeds(embed.Build())
	resp.AddContainerComponents(discord.NewActionRow(discord.NewLinkButton("Jump to message", url)))

	return e.CreateMessage(resp.Build())
}

func userCanReadChannelMessages(userID, channelID snowflake.ID, client bot.Client) (bool, error) {
	fmt.Println("Checking permissions")
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

func respondWithContentEph(e *handler.CommandEvent, content string) error {
	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(content).
		SetEphemeral(true).
		Build(),
	)
}

package ban

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Route(
		"/ban", func(r handler.Router) {
			r.Command("/with-message", BanWithMessageHandler)
			r.Command("/until", BanUntilHandler)
		},
	)

	return []discord.ApplicationCommandCreate{BanCommand}
}

var BanCommand = discord.SlashCommandCreate{
	Name:                     "ban",
	Description:              "Ban a user from the server",
	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionBanMembers),
	Options: []discord.ApplicationCommandOption{
		banWithMessageSubCommand,
		banUntilSubcommand,
	},
}

type BanHandlerData struct {
	User *discord.User
	// BanningUserID is used when parsing the ID, when there is no REST client
	// available.
	BanningUserID snowflake.ID
	BanningUser   *discord.User
	Guild         *discord.Guild
	Duration      string
	Reason        string
	Message       string
}

func (data BanHandlerData) String() string {
	content := fmt.Sprintf(
		"%s\n\x1F\x1F\x1F\nBanned by: %s (%s)",
		data.Reason,
		data.BanningUser.Username,
		data.BanningUserID,
	)

	if data.Duration != "" {
		content += fmt.Sprintf("\nDuration: %s", data.Duration)
	}

	if data.Reason != "" && data.Message != "" {
		content += fmt.Sprintf("\nMessage: %s", data.Duration)
	}

	return content
}

func BanHandlerDataFromString(s string) (data BanHandlerData) {
	reasonSplit := strings.Split(s, "\n\x1F\x1F\x1F\n")
	data.Reason = reasonSplit[0]

	if len(reasonSplit) < 2 {
		return
	}

	trailers := strings.Split(reasonSplit[1], "\n")

	for _, trailer := range trailers {
		parts := strings.SplitN(trailer, ":", 2)

		if len(parts) != 2 {
			continue
		}

		key := strings.Trim(parts[0], " ")
		key = strings.ToLower(key)

		value := strings.Trim(parts[1], " ")
		value = strings.ToLower(value)

		switch key {
		case "duration":
			data.Duration = value
		case "message":
			data.Message = value
		case "banned by":
			re := regexp.MustCompile(`\((\d+)\)`)
			id := re.FindStringSubmatch(value)
			if len(id) < 2 {
				break
			}
			data.BanningUserID = snowflake.MustParse(id[1])
		}
	}

	return
}

func banHandlerInner(e *handler.CommandEvent, data BanHandlerData) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	failedToMessage := false
	if data.Message != "" || data.Duration != "" {
		mc := createBanDMMessage(data)
		_, err := interactions.SendDirectMessage(e.Client(), *data.User, mc)
		if err != nil {
			failedToMessage = true
		}
	}

	err := e.Client().Rest().AddBan(
		guild.ID, data.User.ID, 0,
		rest.WithReason(data.String()),
	)
	if err != nil {
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to ban User").Build())
	}
	if failedToMessage {
		return e.CreateMessage(interactions.EphemeralMessageContent("User was banned but message failed to send.").Build())
	}

	return e.CreateMessage(interactions.EphemeralMessageContent("User was banned.").Build())

}

func createBanDMMessage(data BanHandlerData) discord.MessageCreate {
	banExp := durationToRelTimestamp(data.Duration)

	expiryText := fmt.Sprintf("This ban will expire %s.", banExp)
	messageText := fmt.Sprintf(
		"Along with the ban, this message was added:\n\n %s\n\n",
		data.Message,
	)

	return discord.NewMessageCreateBuilder().
		SetContentf(
			"You have been banned from %s.\n%s%s\n\n(You cannot respond to this message)",
			data.Guild.Name,
			utils.Iif(data.Duration != "", expiryText, ""),
			utils.Iif(data.Message != "", messageText, ""),
		).Build()
}

func durationToRelTimestamp(duration string) string {
	dur, err := utils.ParseLongDuration(duration)
	if err != nil {
		return duration
	}
	return fmt.Sprintf("<t:%d:R>", time.Now().Add(dur).Unix())
}

package ban

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/ban", BanHandler)
	return []discord.ApplicationCommandCreate{BanCommand}
}

var BanCommand = discord.SlashCommandCreate{
	Name:                     "ban",
	Description:              "Ban a user from the server",
	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionBanMembers),
	Options: []discord.ApplicationCommandOption{

		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to ban",
			Required:    true,
		},
		discord.ApplicationCommandOptionString{
			Name:        "reason",
			Description: "Reason for banning the user. Not sent to the user. (Defaults to message)",
			Required:    false,
		},
		discord.ApplicationCommandOptionString{
			Name:        "message",
			Description: "The message sent to the user when banned.",
			Required:    false,
		},
		discord.ApplicationCommandOptionString{
			Name:        "duration",
			Description: "The duration to ban the user for",
			Required:    false,
			Choices:     durationChoices,
		},
		discord.ApplicationCommandOptionInt{
			Name:        "delete-messages",
			Description: "Whether to delete recent messages when banning the user",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceInt{
				{
					Name:  "Do not delete messages",
					Value: 0,
				},
				{
					Name:  "Last 15 minutes",
					Value: 15 * 60,
				},
				{
					Name:  "Last 30 minutes",
					Value: 30 * 60,
				},
				{
					Name:  "Last hour",
					Value: 60 * 60,
				},
				{
					Name:  "Last 2 hours",
					Value: 2 * 60 * 60,
				},
				{
					Name:  "Last 4 hours",
					Value: 4 * 60 * 60,
				},
				{
					Name:  "Last 12 hours",
					Value: 12 * 60 * 60,
				},
				{
					Name:  "Last 24 hours",
					Value: 24 * 60 * 60,
				},
				{
					Name:  "Last 2 days",
					Value: 2 * 24 * 60 * 60,
				},
				{
					Name:  "Last week",
					Value: 7 * 24 * 60 * 60,
				},
			},
		},
	},
}

func BanHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ban", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	data := e.SlashCommandInteractionData()
	user := data.User("user")
	banningUser := e.User()

	duration := data.String("duration")
	reason := data.String("reason")
	message := data.String("message")
	deleteMessages := time.Duration(data.Int("delete-messages")) * time.Second

	if reason == "" {
		reason = message
	}

	banData := BanHandlerData{
		User:          &user,
		BanningUserID: banningUser.ID,
		BanningUser:   &banningUser,
		Guild:         &guild,
		Duration:      duration,
		Reason:        reason,
		Message:       message,
	}

	failedToMessage := false
	if banData.Message != "" || banData.Duration != "" {
		mc := createBanDMMessage(banData)
		_, err := interactions.SendDirectMessage(e.Client(), *banData.User, mc)
		if err != nil {
			slog.Info(
				"Could not DM user with ban information",
				"user", user, "err", err,
			)
			failedToMessage = true
		}
	}

	err := e.Client().Rest().AddBan(
		guild.ID, banData.User.ID, deleteMessages,
		rest.WithReason(banData.String()),
	)
	if err != nil {
		_ = e.CreateMessage(interactions.EphemeralMessageContent("Failed to ban User").Build())
	} else if failedToMessage {
		_ = e.CreateMessage(interactions.EphemeralMessageContent("User was banned but message failed to send.").Build())
	} else {
		_ = e.CreateMessage(interactions.EphemeralMessageContent("User was banned.").Build())
	}

	dur, err := utils.ParseLongDuration(duration)
	if err != nil {
		return err
	}

	_, err = model.CreateTempBan(*e.GuildID(), user.ID, e.User().ID, reason, time.Now().Add(dur))
	if err != nil {
		return err
	}

	return nil
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

	if data.Message != "" {
		content += fmt.Sprintf("\nMessage: %s", data.Message)
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

var durationChoices = []discord.ApplicationCommandOptionChoiceString{
	{
		Name:  "1 week",
		Value: "1w",
	},
	{
		Name:  "2 weeks",
		Value: "2w",
	},
	{
		Name:  "1 month",
		Value: "1mo",
	},
	{
		Name:  "3 months",
		Value: "3mo",
	},
	{
		Name:  "6 months",
		Value: "6mo",
	},
	{
		Name:  "9 months",
		Value: "9mo",
	},
	{
		Name:  "1 year",
		Value: "1y",
	},
	{
		Name:  "2 years",
		Value: "2y",
	},
	{
		Name:  "3 years",
		Value: "3y",
	},
}

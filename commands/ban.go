package commands

import (
	"fmt"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
)

var BanCommand = discord.SlashCommandCreate{
	Name:                     "ban",
	Description:              "Ban a user from the server",
	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionBanMembers),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "with-message",
			Description: "Ban a user, sending a message immediately before the ban",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionUser{
					Name:        "user",
					Description: "The user to ban",
					Required:    true,
				},
				discord.ApplicationCommandOptionString{
					Name:        "message",
					Description: "The message to give the user before banning them (also used as ban reason)",
					Required:    true,
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "until",
			Description: "Ban a user from the server for a specified amount of time",
			Options: []discord.ApplicationCommandOption{

				discord.ApplicationCommandOptionUser{
					Name:        "user",
					Description: "The user to ban",
					Required:    true,
				},
				discord.ApplicationCommandOptionString{
					Name:        "duration",
					Description: "The duration to ban the user for",
					Required:    true,
					Choices:     durationChoices,
				},
				discord.ApplicationCommandOptionString{
					Name:        "reason",
					Description: "Reason for banning the user",
					Required:    false,
				},
				discord.ApplicationCommandOptionBool{
					Name:        "send-reason",
					Description: "Attempt to send the reason to the user as a DM",
					Required:    false,
				},
			},
		},
	},
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

func BanWithMessageHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ban with-message", e)

	data := e.SlashCommandInteractionData()
	user := data.User("user")
	message := data.String("message")

	return banHandlerInner(e, user, true, message, "")
}

func BanUntilHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ban until", e)

	data := e.SlashCommandInteractionData()
	user := data.User("user")
	duration := data.String("duration")
	reason := data.String("reason")
	sendReason := data.Bool("send-reason")

	err := banHandlerInner(e, user, sendReason, reason, duration)
	if err != nil {
		return err
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

func banHandlerInner(e *handler.CommandEvent, user discord.User, sendReason bool, reason, duration string) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	failedToMessage := false
	if sendReason || duration != "" {
		mc := discord.NewMessageCreateBuilder().
			SetContentf(
				"You have been banned from %s.\n"+
					utils.Iif(duration != "", fmt.Sprintf("This ban will expire in %s.\n", duration), "")+
					utils.Iif(sendReason,
						fmt.Sprintf("Along with the ban, this message was added:\n\n %s\n\n", reason), "")+
					"(You cannot respond to this message.)",
				guild.Name,
			).Build()

		_, err := SendDirectMessage(e.Client(), user, mc)
		if err != nil {
			failedToMessage = true
		}
	}

	err := e.Client().Rest().AddBan(guild.ID, user.ID, 0,
		rest.WithReason(fmt.Sprintf("Banned by: %s (%s) %s, with message: %s",
			e.User().Username, e.User().ID,
			utils.Iif(duration != "", fmt.Sprintf("for %s", duration), ""),
			reason,
		)))
	if err != nil {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Failed to ban user").
				Build())
	}

	if failedToMessage {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("User was banned but message failed to send.").
				Build())
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("User was banned.").
			Build())

}

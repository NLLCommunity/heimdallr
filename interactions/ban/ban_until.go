package ban

import (
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var banUntilSubcommand = discord.ApplicationCommandOptionSubCommand{
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
}

func BanUntilHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("ban", e)

	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	data := e.SlashCommandInteractionData()
	user := data.User("user")
	duration := data.String("duration")
	reason := data.String("reason")
	sendReason := data.Bool("send-reason")

	banData := banHandlerData{
		user:       &user,
		guild:      &guild,
		duration:   duration,
		reason:     reason,
		sendReason: sendReason,
	}

	err := banHandlerInner(e, banData)
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

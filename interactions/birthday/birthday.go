package birthday

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
)

type leapDayRouteVars struct {
	UserID snowflake.ID        `rave:"userID"`
	Year   int                 `rave:"year"`
	Policy model.LeapDayPolicy `rave:"policy"`
}

var leapDayRoute = rave.ComponentOf[leapDayRouteVars](
	"/birthday/leap/{userID}/{year}/{policy}",
).Handle(BirthdayLeapDayHandler)

var Interactions = rave.Bundle(Birthday, leapDayRoute)

var Birthday = rave.Slash("birthday", "Set or remove your birthday.").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	AddOptions(
		rave.SubCommand("set", "Set your birthday privately.").
			AddOptions(
				rave.OptionInt("day", "Day of the month.").
					WithRequired(true).
					WithMinValue(1).
					WithMaxValue(31),
				rave.OptionInt("month", "Month of the year.").
					WithRequired(true).
					WithMinValue(1).
					WithMaxValue(12),
				rave.OptionInt("year", "Birth year (optional).").
					WithMinValue(1000).
					WithMaxValue(time.Now().UTC().Year()),
			).
			Handle(BirthdaySetHandler),
		rave.SubCommand("status", "Show your saved birthday privately.").
			Handle(BirthdayStatusHandler),
		rave.SubCommand("remove", "Remove your saved birthday.").
			Handle(BirthdayRemoveHandler),
	)

func buildLeapDayPrompt(userID snowflake.ID, birthYear *int) (discord.MessageCreate, error) {
	year := 0
	if birthYear != nil {
		year = *birthYear
	}
	february28ID, err := leapDayRoute.CustomID(leapDayRouteVars{
		UserID: userID,
		Year:   year,
		Policy: model.LeapDayFebruary28,
	})
	if err != nil {
		return discord.MessageCreate{}, err
	}
	march1ID, err := leapDayRoute.CustomID(leapDayRouteVars{
		UserID: userID,
		Year:   year,
		Policy: model.LeapDayMarch1,
	})
	if err != nil {
		return discord.MessageCreate{}, err
	}

	return ix.EphemeralMessageContent(
		"In years without February 29, when should your birthday be announced?",
	).AddActionRow(
		discord.NewPrimaryButton("February 28", february28ID),
		discord.NewPrimaryButton("March 1", march1ID),
	), nil
}

func BirthdaySetHandler(event *handler.CommandEvent) error {
	guildID, ok := birthdayGuildID(event.GuildID())
	if !ok {
		return ix.ErrEventNoGuildID
	}
	data := event.SlashCommandInteractionData()
	day := data.Int("day")
	month := time.Month(data.Int("month"))
	var birthYear *int
	if year, present := data.OptInt("year"); present {
		birthYear = &year
	}

	if month == time.February && day == 29 {
		if err := model.ValidateBirthday(day, month, birthYear, model.LeapDayFebruary28, time.Now().UTC().Year()); err != nil {
			return event.CreateMessage(ix.EphemeralMessageContent(err.Error()))
		}
		message, err := buildLeapDayPrompt(event.User().ID, birthYear)
		if err != nil {
			return err
		}
		return event.CreateMessage(message)
	}

	_, err := model.SetBirthday(guildID, event.User().ID, day, month, birthYear, "", time.Now().UTC().Year())
	if err != nil {
		return event.CreateMessage(ix.EphemeralMessageContent(err.Error()))
	}
	return event.CreateMessage(ix.EphemeralMessageContent("Your birthday has been saved."))
}

func BirthdayStatusHandler(event *handler.CommandEvent) error {
	guildID, ok := birthdayGuildID(event.GuildID())
	if !ok {
		return ix.ErrEventNoGuildID
	}
	birthday, err := model.GetBirthday(guildID, event.User().ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return event.CreateMessage(ix.EphemeralMessageContent("You do not have a birthday saved."))
	}
	if err != nil {
		return err
	}

	date := fmt.Sprintf("%d %s", birthday.Day, birthday.Month)
	if birthday.BirthYear != nil {
		date += fmt.Sprintf(" %d", *birthday.BirthYear)
	}
	if birthday.Day == 29 && birthday.Month == time.February {
		switch birthday.LeapDayPolicy {
		case model.LeapDayFebruary28:
			date += " (February 28 in non-leap years)"
		case model.LeapDayMarch1:
			date += " (March 1 in non-leap years)"
		}
	}
	return event.CreateMessage(ix.EphemeralMessageContent("Your saved birthday is " + date + "."))
}

func BirthdayRemoveHandler(event *handler.CommandEvent) error {
	guildID, ok := birthdayGuildID(event.GuildID())
	if !ok {
		return ix.ErrEventNoGuildID
	}
	if err := model.DeleteBirthday(guildID, event.User().ID); err != nil {
		return err
	}
	return event.CreateMessage(ix.EphemeralMessageContent("Your birthday has been removed."))
}

func BirthdayLeapDayHandler(event *handler.ComponentEvent) error {
	guildID, ok := birthdayGuildID(event.GuildID())
	if !ok {
		return ix.ErrEventNoGuildID
	}
	userID, err := snowflake.Parse(event.Vars["userID"])
	if err != nil {
		return event.CreateMessage(ix.EphemeralMessageContent("This birthday choice is invalid."))
	}
	if userID != event.User().ID {
		return event.CreateMessage(ix.EphemeralMessageContent("You can only choose a policy for your own birthday."))
	}
	year, err := strconv.Atoi(event.Vars["year"])
	if err != nil || year < 0 {
		return event.CreateMessage(ix.EphemeralMessageContent("This birthday choice is invalid."))
	}
	policy := model.LeapDayPolicy(event.Vars["policy"])
	if policy != model.LeapDayFebruary28 && policy != model.LeapDayMarch1 {
		return event.CreateMessage(ix.EphemeralMessageContent("This birthday choice is invalid."))
	}
	var birthYear *int
	if year != 0 {
		birthYear = &year
	}
	if _, err := model.SetBirthday(
		guildID,
		userID,
		29,
		time.February,
		birthYear,
		policy,
		time.Now().UTC().Year(),
	); err != nil {
		return event.CreateMessage(ix.EphemeralMessageContent(err.Error()))
	}
	return event.UpdateMessage(
		discord.NewMessageUpdate().
			WithContent("Your birthday has been saved.").
			ClearComponents(),
	)
}

func birthdayGuildID(guildID *snowflake.ID) (snowflake.ID, bool) {
	if guildID == nil {
		return 0, false
	}
	return *guildID, true
}

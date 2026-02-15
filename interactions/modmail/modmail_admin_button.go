package modmail

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

var createSubcommand = discord.ApplicationCommandOptionSubCommand{
	Name:        "create-button",
	Description: "Create a button for creating Modmail threads in the current channel.",
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionString{
			Name:        "label",
			Description: "The label to display on the button.",
			Required:    true,
			MinLength:   utils.Ref(3),
		},
		discord.ApplicationCommandOptionString{
			Name:        "button-color",
			Description: "The color of the button.",
			Required:    false,
			Choices: []discord.ApplicationCommandOptionChoiceString{
				{Name: "Red", Value: "red"},
				{Name: "Green", Value: "green"},
				{Name: "Blue", Value: "blue"},
				{Name: "Gray", Value: "gray"},
			},
		},
		discord.ApplicationCommandOptionRole{
			Name:        "role",
			Description: "Role that should be mentioned/notified when a new thread is created.",
			Required:    false,
		},
		discord.ApplicationCommandOptionChannel{
			Name:        "channel",
			Description: "Channel that notifications should be sent to.",
			Required:    false,
			ChannelTypes: []discord.ChannelType{
				discord.ChannelTypeGuildText,
			},
		},
		discord.ApplicationCommandOptionInt{
			Name:        "max-active-reports",
			Description: "The maximum number of active reports that a user can have in the channel.",
			Required:    false,
			MinValue:    utils.Ref(0),
			MaxValue:    utils.Ref(100),
		},
		discord.ApplicationCommandOptionString{
			Name:        "slow-mode-time",
			Description: "Enable slow mode for the report thread in the format '1h5m30s1' ('0s' = disabled)",
			Required:    false,
		},
	},
}

var stringToButtonStyle = map[string]discord.ButtonStyle{
	"red":   discord.ButtonStyleDanger,
	"green": discord.ButtonStyleSuccess,
	"blue":  discord.ButtonStylePrimary,
	"gray":  discord.ButtonStyleSecondary,
}

func ModmailAdminCreateButtonHandler(e *handler.CommandEvent) error {
	utils.LogInteraction("modmail", e)

	data := e.SlashCommandInteractionData()
	label := data.String("label")
	color := data.String("button-color")
	role := data.Role("role")
	channel := data.Channel("channel")
	maxActive := data.Int("max-active-reports")
	slowModeStr, slowModeOK := data.OptString("slow-mode-time")
	if !slowModeOK {
		slowModeStr = "0s"
	}
	if color == "" {
		color = "blue"
	}

	slowMode, err := time.ParseDuration(slowModeStr)
	if err != nil {
		slog.Info(
			"Failed to parse slow mode duration",
			"slow_mode", slowModeStr, "err", err,
		)
	}

	if slowMode.Hours() > 6 {
		return e.CreateMessage(
			ix.EphemeralMessageContentf(
				"Slow mode duration is too long '%s'. Max is six hours.",
				slowModeStr,
			),
		)
	}

	return e.CreateMessage(
		discord.NewMessageCreate().
			AddActionRow(
				discord.NewButton(
					stringToButtonStyle[color],
					label,
					fmt.Sprintf(
						"/modmail/report-button/%s/%s/%d/%.0f",
						role.ID, channel.ID, maxActive, slowMode.Seconds(),
					),
					"", 0,
				),
			),
	)
}

func ModmailReportButtonHandler(e *handler.ComponentEvent) error {
	utils.LogInteraction("modmail", e)

	role := e.Vars["role"]
	channel := e.Vars["channel"]
	maxActiveStr := e.Vars["max-active"]
	slowModeStr := e.Vars["slow-mode"]

	maxActive, err := strconv.Atoi(maxActiveStr)
	if err != nil {
		slog.Error("Failed to parse max active")
		return e.CreateMessage(
			ix.EphemeralMessageContent("Failed to create report modal"),
		)
	}

	below, err := isBelowMaxActive(e, maxActive)
	if err != nil {
		return e.CreateMessage(
			ix.EphemeralMessageContent("Something went wrong when preparing for the report."),
		)
	}
	if !below {
		return e.CreateMessage(
			ix.EphemeralMessageContent("You already have the maximum number of reports open"),
		)
	}

	customID := fmt.Sprintf("/modmail/report-modal/%s/%s/%s/%s", role, channel, maxActiveStr, slowModeStr)

	slog.Info("Sending modal", "custom_id", customID)

	modal := discord.NewModalCreate(customID, "Report", nil).
		AddLabel(
			"Subject", discord.NewShortTextInput("title").
				WithPlaceholder("Subject or topic of the report").
				WithRequired(true).
				WithMinLength(5).
				WithMaxLength(100),
		).
		AddLabel(
			"Description", discord.NewParagraphTextInput("description").
				WithPlaceholder(
					"Report information\n\n"+
						"Markdown is supported\n\n"+
						"More details, imager, etc. can be submitted afterwards",
				).
				WithRequired(true).
				WithMinLength(10),
		)

	err = e.Modal(modal)
	if err != nil {
		slog.Error("Failed to send modal", "err", err)
		return err
	}

	slog.Info("Sent modal")
	return nil
}

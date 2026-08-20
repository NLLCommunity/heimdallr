package modmail

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

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

	customID, err := modmailReportButtonRoute.CustomID(modmailReportVars{
		Role:      role.ID,
		Channel:   channel.ID,
		MaxActive: maxActive,
		SlowMode:  slowModeCustomIDValue(slowMode),
	})
	if err != nil {
		return err
	}

	return e.CreateMessage(
		discord.NewMessageCreate().
			AddActionRow(
				discord.NewButton(
					stringToButtonStyle[color],
					label,
					customID,
					"", 0,
				),
			),
	)
}

func slowModeCustomIDValue(slowMode time.Duration) string {
	return strconv.FormatFloat(slowMode.Seconds(), 'f', 0, 64)
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

	customID, err := modmailReportModalRoute.CustomIDVars(rave.Vars{
		"role": role, "channel": channel, "max-active": maxActiveStr, "slow-mode": slowModeStr,
	})
	if err != nil {
		return err
	}

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

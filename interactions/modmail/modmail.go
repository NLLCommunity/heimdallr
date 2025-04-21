package modmail

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/modmail-admin/create-button", ModmailAdminCreateButtonHandler)
	r.Component("/modmail/report-button/{role}/{channel}/{max-active}/{slow-mode}", ModmailReportButtonHandler)
	r.Modal("/modmail/report-modal/{role}/{channel}/{max-active}/{slow-mode}", ModmailReportModalHandler)

	return []discord.ApplicationCommandCreate{ModmailCommand}
}

var ModmailCommand = discord.SlashCommandCreate{
	Name:                     "modmail-admin",
	Description:              "Commands for receiving and sending Modmail.",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Contexts: []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	},
	Options: []discord.ApplicationCommandOption{
		createSubcommand,
	},
}

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
	slowModeStr := data.String("slow-mode-time")
	if color == "" {
		color = "blue"
	}

	slowMode, err := time.ParseDuration(slowModeStr)
	if err != nil {
		slog.Info("Failed to parse slow mode duration",
			"slow_mode", slowModeStr, "err", err)
	}

	if slowMode.Hours() > 6 {
		return e.CreateMessage(
			ix.EphemeralMessageContentf("Slow mode duration is too long '%s'. Max is six hours.",
				slowModeStr).Build())
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			AddActionRow(discord.NewButton(
				stringToButtonStyle[color],
				label,
				fmt.Sprintf("/modmail/report-button/%s/%s/%d/%.0f",
					role.ID, channel.ID, maxActive, slowMode.Seconds()),
				"", 0,
			)).Build(),
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
			ix.EphemeralMessageContent("Failed to create report modal").
				Build())
	}

	below, err := isBelowMaxActive(e, maxActive)
	if err != nil {
		return e.CreateMessage(
			ix.EphemeralMessageContent("Something went wrong when preparing for the report.").
				Build())
	}
	if !below {
		return e.CreateMessage(
			ix.EphemeralMessageContent("You already have the maximum number of reports open").
				Build())
	}

	customID := fmt.Sprintf("/modmail/report-modal/%s/%s/%s/%s", role, channel, maxActiveStr, slowModeStr)

	slog.Info("Sending modal", "custom_id", customID)

	modal := discord.NewModalCreateBuilder().
		SetCustomID(customID).
		SetTitle("Report").
		AddActionRow(
			discord.NewShortTextInput("title", "Subject").
				WithPlaceholder("Subject or topic of the report").
				WithRequired(true).
				WithMinLength(5).
				WithMaxLength(100)).
		AddActionRow(
			discord.NewParagraphTextInput("description", "Description").
				WithPlaceholder(
					"Report information\n\n" +
						"Markdown is supported\n\n" +
						"More details, imager, etc. can be submitted afterwards"),
		).
		Build()

	err = e.Modal(modal)
	if err != nil {
		slog.Error("Failed to send modal", "err", err)
		return err
	}

	slog.Info("Sent modal")
	return nil
}

func ModmailReportModalHandler(e *handler.ModalEvent) error {
	_ = e.DeferCreateMessage(true)

	role := e.Vars["role"]
	channel := e.Vars["channel"]
	maxActiveStr := e.Vars["max-active"]
	slowModeStr := e.Vars["slow-mode"]

	maxActive, err := strconv.Atoi(maxActiveStr)
	if err != nil {
		slog.Warn("Failed to parse 'max active'.", "max_active", maxActiveStr, "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report").
				Build())
		return err
	}

	slowMode, err := strconv.Atoi(slowModeStr)
	if err != nil {
		slog.Error("Failed to parse 'slow mode'.", "slow_mode", slowModeStr, "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report").
				Build())
		return err
	}

	canSubmit, err := isBelowMaxActive(e, maxActive)
	if err != nil {
		slog.Error("Failed to check if user can submit report",
			"max-active", maxActiveStr, "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report.").
				Build())
		return err
	}

	if !canSubmit {
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("You have reached the maximum number of active reports.").
				Build())
		return err
	}

	title := e.Data.Text("title")
	description := e.Data.Text("description")

	thread, err := e.Client().Rest().CreateThread(
		e.Channel().ID(),
		discord.GuildPrivateThreadCreate{
			Name:                title,
			AutoArchiveDuration: 10080,
			Invitable:           utils.Ref(false),
		})
	if err != nil {
		slog.Error("Failed to create thread", "err", err)
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Failed to submit report.").
				Build())
		return err
	}

	if slowMode > 0 {
		_, err := e.Client().Rest().UpdateChannel(thread.ID(), discord.GuildThreadUpdate{
			RateLimitPerUser: utils.Ref(slowMode),
		})
		if err != nil {
			slog.Error("Failed to update thread rate limit", "err", err, "channel", thread.ID())
		}
	}

	user := e.User()
	avatarURL := getUserAvatarURL(&user)

	embed := discord.NewEmbedBuilder().
		SetTitle(title).
		SetDescription(description).
		SetColor(0x4848FF).
		SetAuthor(user.Username, "", avatarURL).
		Build()

	message, err := e.Client().Rest().CreateMessage(
		thread.ID(),
		discord.MessageCreate{
			Content: fmt.Sprintf("||%s%s",
				utils.Iif(role != "0", fmt.Sprintf("<@&%s>", role), ""),
				user.Mention()),
			Embeds: []discord.Embed{embed},
		})
	if err != nil {
		return err
	}

	if channel != "" && channel != "0" {
		channelSnowflake, err := snowflake.Parse(channel)
		if err == nil {
			_, err = e.Client().Rest().CreateMessage(channelSnowflake,
				discord.NewMessageCreateBuilder().
					SetContentf("### New Modmail thread in <#%d>", e.Channel().ID()).
					AddEmbeds(embed).
					AddActionRow(
						discord.NewLinkButton("Go to thread", message.JumpURL()),
					).Build())

			if err != nil {
				slog.Error("Failed to send message to Modmail notification channel",
					"err", err, "channel", channel)
			}
		}
	}

	_, err = e.CreateFollowupMessage(
		ix.EphemeralMessageContent("Report created!").
			AddActionRow(discord.NewLinkButton("View", message.JumpURL())).
			Build())

	return err
}

func getUserAvatarURL(user *discord.User) string {
	avatarURL := user.AvatarURL()
	if avatarURL != nil {
		return *avatarURL
	}

	return user.DefaultAvatarURL()
}

func isBelowMaxActive(e interactionEvent, maxActive int) (bool, error) {
	if maxActive == 0 {
		return true, nil
	}

	if e.GuildID() == nil {
		slog.Error("Cannot determine if below max active modmails: no guild")
		return false, ix.ErrEventNoGuildID
	}

	guildID := *e.GuildID()

	activeThreads, err := e.Client().Rest().GetActiveGuildThreads(guildID)
	if err != nil {
		slog.Error("Failed to retrieve active threads", "err", err)
		return false, fmt.Errorf("unable to retrieve active guild threads: %w", err)
	}

	userThreadsCount := 0

	for _, thread := range activeThreads.Threads {
		if *thread.ParentID() != e.Channel().ID() {
			continue
		}
		members, err := e.Client().Rest().GetThreadMembers(thread.ID())
		if err != nil {
			slog.Error("Failed to get thread members", "err", err)
			return false, fmt.Errorf("couldn't get thread members: %w", err)
		}

		for _, member := range members {
			if member.UserID == e.User().ID {
				userThreadsCount++
			}
			if userThreadsCount >= maxActive {
				return false, nil
			}
		}
	}

	if userThreadsCount >= maxActive {
		return false, nil
	}

	return true, nil
}

type interactionEvent interface {
	Channel() discord.InteractionChannel
	Client() bot.Client
	GuildID() *snowflake.ID
	User() discord.User
}

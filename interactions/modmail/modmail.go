package modmail

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	ix "github.com/NLLCommunity/heimdallr/interactions"
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

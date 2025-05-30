package gatekeep

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var activeApprovalProcesses = make(map[snowflake.ID]bool)
var activeApprovalMutex = &sync.Mutex{}

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/approve", ApproveSlashCommandHandler)
	r.Command("/Approve", ApproveUserCommandHandler)

	return []discord.ApplicationCommandCreate{ApproveSlashCommand, ApproveUserCommand}
}

func getGuild(e *handler.CommandEvent) (guild discord.Guild, success bool, inGuild bool) {
	if e.GuildID() == nil {
		return
	}
	inGuild = true
	guild, success = e.Guild()

	if success {
		return
	}

	restGuild, err := e.Client().Rest().GetGuild(*e.GuildID(), false)
	if err != nil {
		slog.Warn("Failed to get guild", "guild_id", *e.GuildID(), "err", err)
		return
	}

	guild = restGuild.Guild
	success = true
	return
}

func approvedInnerHandler(e *handler.CommandEvent, guild discord.Guild, member discord.ResolvedMember) error {
	// Ensure that the user is not already being approved
	// by another command invocation.
	activeApprovalMutex.Lock()
	if activeApprovalProcesses[member.User.ID] {
		activeApprovalMutex.Unlock()
		return e.CreateMessage(
			interactions.EphemeralMessageContentf(
				"%s is already being approved.", member.Mention(),
			).Build(),
		)
	}
	activeApprovalProcesses[member.User.ID] = true
	activeApprovalMutex.Unlock()

	defer func() {
		activeApprovalMutex.Lock()
		delete(activeApprovalProcesses, member.User.ID)
		activeApprovalMutex.Unlock()
	}()

	slog.InfoContext(e.Ctx, "Entered approvedInnerHandler")
	err := e.DeferCreateMessage(true)
	if err != nil {
		slog.Error("Failed to defer message.", "err", err)
	}

	guildSettings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		slog.ErrorContext(
			e.Ctx, "Failed to get guild settings.",
			"guild_id", guild.ID,
			"err", err,
		)
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to get guild information.").Build())
	}

	if !guildSettings.GatekeepEnabled {
		return e.CreateMessage(interactions.EphemeralMessageContent("Gatekeep is not enabled in this server.").Build())
	}

	hasApprovedRole := false
	hasPendingRole := false
	for _, roleID := range member.RoleIDs {
		switch roleID {
		case guildSettings.GatekeepApprovedRole:
			hasApprovedRole = true
		case guildSettings.GatekeepPendingRole:
			hasPendingRole = true
		}
	}

	if hasApprovedRole && (!hasPendingRole || !guildSettings.GatekeepAddPendingRoleOnJoin) {
		return e.CreateMessage(
			interactions.EphemeralMessageContentf(
				"User %s is already approved.", member.Mention(),
			).Build(),
		)
	}

	if guildSettings.GatekeepApprovedRole != 0 {
		err = e.Client().Rest().AddMemberRole(
			guild.ID, member.User.ID,
			guildSettings.GatekeepApprovedRole,
			rest.WithReason(fmt.Sprintf("Gatekeep approved by: %s (%s)", e.User().Username, e.User().ID)),
		)
		if err != nil {
			slog.Warn(
				"Failed to add approved role to user",
				"guild_id", guild.ID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepApprovedRole,
			)
			return err
		}
	}
	if guildSettings.GatekeepPendingRole != 0 {
		err = e.Client().Rest().RemoveMemberRole(
			guild.ID, member.User.ID,
			guildSettings.GatekeepPendingRole,
			rest.WithReason(fmt.Sprintf("Gatekeep approved by: %s (%s)", e.User().Username, e.User().ID)),
		)
		if err != nil {
			slog.Warn(
				"Failed to remove pending role from user",
				"guild_id", guild.ID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepPendingRole,
			)
			return err
		}
	}

	slog.InfoContext(
		e.Ctx, "user has been approved",
		"guild_id", guild.ID,
	)

	if guildSettings.GatekeepApprovedMessage == "" {
		slog.Info("No approved message set; not sending message.")
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"No approved message set; not sending message. Roles have been set.").
			Build())
	}

	channel := guildSettings.JoinLeaveChannel
	if channel == 0 {
		if guild.SystemChannelID != nil {
			channel = *guild.SystemChannelID
		}
	}

	templateData := utils.NewMessageTemplateData(member.Member, guild)
	contents, err := mustache.RenderRaw(guildSettings.GatekeepApprovedMessage, true, templateData)
	if err != nil {
		slog.Warn("Failed to render approved message template.")
		return err
	}
	_, err = e.Client().Rest().CreateMessage(
		channel,
		discord.NewMessageCreateBuilder().
			SetContent(
				contents+
					fmt.Sprintf("\n\n-# Approved by %s", e.User().Mention()),
			).
			SetAllowedMentions(
				&discord.AllowedMentions{
					Users: []snowflake.ID{member.User.ID},
				},
			).
			Build(),
	)
	if err != nil {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"Failed to send message to approved user.").Build())
	}

	_, err = e.CreateFollowupMessage(interactions.EphemeralMessageContent(
		"User has been approved!").Build())
	return err
}

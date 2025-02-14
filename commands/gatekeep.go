package commands

import (
	"fmt"
	"log/slog"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

var ApproveUserCommand = discord.UserCommandCreate{
	Name:                     "Approve",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Contexts: []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	},
}

var ApproveSlashCommand = discord.SlashCommandCreate{
	Name:                     "approve",
	Description:              "Approve a user to join the server",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	Contexts: []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	},

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to approve",
			Required:    true,
		},
	},
}

func ApproveUserCommandHandler(e *handler.CommandEvent) error {
	utils.LogInteractionContext("approve", e, e.Ctx)

	guild, success, inGuild := getGuild(e)
	if !inGuild {
		slog.Warn("approve command supplied in DMs or guild ID is otherwise nil")
		return nil
	}
	if !success {
		slog.Warn("approve command: failed to get guild")
		return nil
	}

	member := e.UserCommandInteractionData().TargetMember()

	return approvedInnerHandler(e, guild, member)
}

func ApproveSlashCommandHandler(e *handler.CommandEvent) error {
	utils.LogInteractionContext("Approve", e, e.Ctx)

	guild, success, inGuild := getGuild(e)
	if !inGuild {
		slog.Warn("approve command supplied in DMs or guild ID is otherwise nil")
		return nil
	}
	if !success {
		slog.Warn("approve command: failed to get guild")
		return nil
	}

	member := e.SlashCommandInteractionData().Member("user")

	return approvedInnerHandler(e, guild, member)
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
	slog.InfoContext(e.Ctx, "Entered approvedInnerHandler")
	err := e.DeferCreateMessage(true)
	if err != nil {
		slog.Error("Failed to defer message.", "err", err)
	}

	guildSettings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		slog.ErrorContext(e.Ctx, "Failed to get guild settings.",
			"guild_id", guild.ID,
			"err", err)
		_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetContent("Failed to get guild information.").
			SetEphemeral(true).
			Build())
		return err
	}

	if !guildSettings.GatekeepEnabled {
		_, err := e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetContentf("Gatekeep is not enabled in this server.").
			SetEphemeral(true).
			Build())
		return err
	}

	hasApprovedRole := false
	hasPendingRole := false
	for _, roleID := range member.RoleIDs {
		if roleID == guildSettings.GatekeepApprovedRole {
			hasApprovedRole = true
		} else if roleID == guildSettings.GatekeepPendingRole {
			hasPendingRole = true
		}
	}

	if hasApprovedRole && (!hasPendingRole || !guildSettings.GatekeepAddPendingRoleOnJoin) {
		_, err := e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetContentf("User %s is already approved.", member.Mention()).
			SetEphemeral(true).
			Build())
		return err

	}

	if guildSettings.GatekeepApprovedRole != 0 {
		err = e.Client().Rest().AddMemberRole(guild.ID, member.User.ID,
			guildSettings.GatekeepApprovedRole,
			rest.WithReason(fmt.Sprintf("Gatekeep approved by: %s (%s)", e.User().Username, e.User().ID)),
		)
		if err != nil {
			slog.Warn("Failed to add approved role to user",
				"guild_id", guild.ID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepApprovedRole)
			return err
		}
	}
	if guildSettings.GatekeepPendingRole != 0 {
		err = e.Client().Rest().RemoveMemberRole(guild.ID, member.User.ID,
			guildSettings.GatekeepPendingRole,
			rest.WithReason(fmt.Sprintf("Gatekeep approved by: %s (%s)", e.User().Username, e.User().ID)),
		)
		if err != nil {
			slog.Warn("Failed to remove pending role from user",
				"guild_id", guild.ID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepPendingRole)
			return err
		}
	}

	slog.InfoContext(e.Ctx, "user has been approved",
		"guild_id", guild.ID)

	if guildSettings.GatekeepApprovedMessage == "" {
		slog.Info("No approved message set; not sending message.")
		_, err := e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetContentf("No approved message set; not sending message. Roles have been set.").
			SetEphemeral(true).
			Build())
		return err
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
	_, err = e.Client().Rest().CreateMessage(channel,
		discord.NewMessageCreateBuilder().
			SetContent(contents+
				fmt.Sprintf("\n\n-# Approved by %s", e.User().Mention())).
			SetAllowedMentions(&discord.AllowedMentions{
				Users: []snowflake.ID{member.User.ID},
			}).
			Build(),
	)
	if err != nil {
		_, err := e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to send message to approved user.").
			Build())
		return err
	}
	_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent("User has been approved!").
		Build())
	return err
}

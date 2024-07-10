package commands

import (
	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
	"log/slog"
)

var ApproveUserCommand = discord.UserCommandCreate{
	Name:                     "approve",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	DMPermission:             utils.Ref(false),
}

var ApproveCommand = discord.SlashCommandCreate{
	Name:                     "approve-user",
	Description:              "Approve a user to join the server",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionKickMembers),
	DMPermission:             utils.Ref(false),

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionUser{
			Name:        "user",
			Description: "The user to approve",
			Required:    true,
		},
	},
}

func ApproveUserHandler(e *handler.CommandEvent) error {
	slog.Info("`approve` user command called.",
		"guild_id", utils.Iif(e.GuildID() == nil, "<null>", e.GuildID().String()))
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	member := e.UserCommandInteractionData().TargetMember()

	return approvedInnerHandler(e, guild, member)
}

func ApproveHandler(e *handler.CommandEvent) error {
	slog.Info("`approve-user` slash command called.",
		"guild_id", utils.Iif(e.GuildID() == nil, "<null>", e.GuildID().String()))
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	member := e.SlashCommandInteractionData().Member("user")

	return approvedInnerHandler(e, guild, member)
}

func approvedInnerHandler(e *handler.CommandEvent, guild discord.Guild, member discord.ResolvedMember) error {
	guildSettings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		slog.Error("Failed to get guild settings.",
			"guild_id", guild.ID,
			"err", err)
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
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContentf("User %s is already approved.", member.Mention()).
			SetEphemeral(true).
			Build())

	}

	if guildSettings.GatekeepApprovedRole != 0 {
		err = e.Client().Rest().AddMemberRole(guild.ID, member.User.ID, guildSettings.GatekeepApprovedRole)
		if err != nil {
			slog.Warn("Failed to add approved role to user",
				"guild_id", guild.ID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepApprovedRole)
			return err
		}
	}
	if guildSettings.GatekeepPendingRole != 0 {
		err = e.Client().Rest().RemoveMemberRole(guild.ID, member.User.ID, guildSettings.GatekeepPendingRole)
		if err != nil {
			slog.Warn("Failed to remove pending role from user",
				"guild_id", guild.ID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepPendingRole)
			return err
		}
	}

	if guildSettings.GatekeepApprovedMessage == "" {
		slog.Info("No approved message set; not sending message.")
		return nil
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
			SetContent(contents).
			SetAllowedMentions(&discord.AllowedMentions{
				Users: []snowflake.ID{member.User.ID},
			}).
			Build(),
	)
	return err
}

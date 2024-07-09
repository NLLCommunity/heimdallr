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
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	member := e.UserCommandInteractionData().TargetMember()

	return approvedInnerHandler(e, guild, member)
}

func ApproveHandler(e *handler.CommandEvent) error {
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	member := e.SlashCommandInteractionData().Member("user")

	return approvedInnerHandler(e, guild, member)
}

func approvedInnerHandler(e *handler.CommandEvent, guild discord.Guild, member discord.ResolvedMember) error {
	guildSettings, err := model.GetGuildSettings(member.GuildID)
	if err != nil {
		return err
	}

	if guildSettings.GatekeepApprovedRole != 0 {
		err = e.Client().Rest().AddMemberRole(member.GuildID, member.User.ID, guildSettings.GatekeepApprovedRole)
		if err != nil {
			slog.Warn("Failed to add approved role to user",
				"guild_id", member.GuildID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepApprovedRole)
		}
	}
	if guildSettings.GatekeepPendingRole != 0 {
		err = e.Client().Rest().RemoveMemberRole(member.GuildID, member.User.ID, guildSettings.GatekeepPendingRole)
		if err != nil {
			slog.Warn("Failed to remove pending role from user",
				"guild_id", member.GuildID,
				"user_id", member.User.ID,
				"role_id", guildSettings.GatekeepPendingRole)
		}
	}

	if guildSettings.GatekeepApprovedMessage == "" {
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
		return err
	}
	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(contents).
		SetAllowedMentions(&discord.AllowedMentions{
			Users: []snowflake.ID{member.User.ID},
		}).
		Build())
}

package commands

import (
	"errors"
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
	"log/slog"
)

var AdminCommand = discord.SlashCommandCreate{
	Name:        "admin",
	Description: "Various admin commands",

	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionAdministrator),
	DMPermission:             utils.Ref(false),

	Options: []discord.ApplicationCommandOption{
		modChannelSubCommandGroup,
		infractionHalfLifeSubCommandGroup,
		notifyOnWarnedUserJoinSubCommandGroup,
		gatekeepSubCommandGroup,
		joinLeaveSubCommandGroup,
	},
}

var modChannelSubCommandGroup = discord.ApplicationCommandOptionSubCommandGroup{
	Name:        "mod-channel",
	Description: "Admin commands relating to the moderator channel",
	Options: []discord.ApplicationCommandOptionSubCommand{
		{
			Name:        "set",
			Description: "Set the moderator channel",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionChannel{
					Name:         "channel",
					Description:  "The channel to set as the moderator channel",
					Required:     true,
					ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
				},
			},
		},
		{
			Name:        "clear",
			Description: "Clear the moderator channel",
		},
		{
			Name:        "get",
			Description: "Get the current moderator channel",
		},
	},
}

var infractionHalfLifeSubCommandGroup = discord.ApplicationCommandOptionSubCommandGroup{
	Name:        "infraction-half-life",
	Description: "Admin commands relating to the infraction half-life",
	Options: []discord.ApplicationCommandOptionSubCommand{
		{
			Name:        "set",
			Description: "Set the half-life time of infractions in days.",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionFloat{
					Name:        "days",
					Description: "The half-life time of infractions in days. (0 = no half-life)",
					Required:    true,
					MinValue:    utils.Ref(0.0),
				},
			},
		},
		{
			Name:        "clear",
			Description: "Clear the half-life time of infractions. Effectively sets it to 0.",
		},
		{
			Name:        "get",
			Description: "Get the current half-life time of infractions.",
		},
	},
}

var notifyOnWarnedUserJoinSubCommandGroup = discord.ApplicationCommandOptionSubCommandGroup{
	Name:        "notify-on-warned-user-join",
	Description: "Admin commands relating to notifying on warned user join",
	Options: []discord.ApplicationCommandOptionSubCommand{
		{
			Name:        "set",
			Description: "Set whether to notify when a warned user joins the server",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "enabled",
					Description: "Sets whether to enable notifications on warned user join",
					Required:    true,
				},
			},
		},
		{
			Name:        "get",
			Description: "Get whether to notify when a warned user joins the server",
		},
	},
}

var gatekeepSubCommandGroup = discord.ApplicationCommandOptionSubCommandGroup{
	Name:        "gatekeep",
	Description: "Admin commands relating to Gatekeep (user approval system)",
	Options: []discord.ApplicationCommandOptionSubCommand{
		{
			Name:        "enabled",
			Description: "Set whether or not to enable Gatekeep",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "enabled",
					Description: "Whether or not to enable Gatekeep",
					Required:    true,
				},
			},
		},
		{
			Name:        "info",
			Description: "Get information about the current Gatekeep settings",
		},
		{
			Name:        "give-pending-role-on-join",
			Description: "Set whether or not to give the pending role to users when they join",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "enabled",
					Description: "Whether or not to give the pending role to users when they join",
					Required:    true,
				},
			},
		},
		{
			Name:        "pending-role",
			Description: "Set the role to give to users pending approval",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionRole{
					Name:        "role",
					Description: "The role to give to users pending approval",
					Required:    false,
				},
			},
		},
		{
			Name:        "approved-role",
			Description: "Set the role to give to approved users",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionRole{
					Name:        "role",
					Description: "The role to give to approved users",
					Required:    false,
				},
			},
		},
		{
			Name:        "approved-message",
			Description: "Set the message to send to approved users",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "message",
					Description: "The message to send to approved users",
					Required:    false,
				},
			},
		},
	},
}

var joinLeaveSubCommandGroup = discord.ApplicationCommandOptionSubCommandGroup{
	Name:        "join-leave",
	Description: "Admin commands relating to join and leave messages",
	Options: []discord.ApplicationCommandOptionSubCommand{
		{
			Name:        "info",
			Description: "Get information about the current join and leave message settings",
		},
		{
			Name:        "join-enabled",
			Description: "Set whether or not to enable join messages",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "enabled",
					Description: "Whether or not to enable join messages",
					Required:    true,
				},
			},
		},
		{
			Name:        "join-message",
			Description: "Set the message to send when a user joins",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "message",
					Description: "The message to send when a user joins",
					Required:    false,
				},
			},
		},
		{
			Name:        "leave-enabled",
			Description: "Set whether or not to enable leave messages",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "enabled",
					Description: "Whether or not to enable leave messages",
					Required:    true,
				},
			},
		},
		{
			Name:        "leave-message",
			Description: "Set the message to send when a user leaves",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "message",
					Description: "The message to send when a user leaves",
					Required:    false,
				},
			},
		},
		{
			Name:        "channel",
			Description: "Set the channel to send join and leave messages",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionChannel{
					Name:         "channel",
					Description:  "The channel to send join and leave messages",
					Required:     true,
					ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
				},
			},
		},
	},
}

func AdminModChannelSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef
	data := e.SlashCommandInteractionData()

	modChannel := data.Channel("channel")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.ModeratorChannel = modChannel.ID
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set moderator channel. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Moderator channel set to <#%d>.", modChannel.ID)).
		Build())
}

func AdminModChannelClearCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.ModeratorChannel = 0
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to clear moderator channel. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent("Moderator channel cleared.").
		Build())
}

func AdminModChannelGetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if guildSettings.ModeratorChannel == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("No moderator channel set.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Moderator channel is set to <#%d>.", guildSettings.ModeratorChannel)).
		Build())
}

func AdminInfractionHalfLifeSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	days := data.Float("days")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.InfractionHalfLifeDays = days
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set infraction half-life. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Infraction half-life set to %.2f days.", days)).
		Build())
}

func AdminInfractionHalfLifeClearCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.InfractionHalfLifeDays = 0
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to clear infraction half-life. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent("Infraction half-life cleared.").
		Build())
}

func AdminInfractionHalfLifeGetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if guildSettings.InfractionHalfLifeDays == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("No infraction half-life set.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Infraction half-life is set to %.2f days.", guildSettings.InfractionHalfLifeDays)).
		Build())
}

func AdminNotifyOnWarnedUserJoinSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	enabled := data.Bool("enabled")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.NotifyOnWarnedUserJoin = enabled
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set notify on warned user join. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Notify on warned user join set to %t.", enabled)).
		Build())
}

func AdminNotifyOnWarnedUserJoinGetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Notify on warned user join is set to %t.", guildSettings.NotifyOnWarnedUserJoin)).
		Build())
}

func AdminGatekeepEnabledCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	enabled := data.Bool("enabled")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.GatekeepEnabled = enabled
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set Gatekeep enabled. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Gatekeep enabled set to %t.", enabled)).
		Build())
}

func AdminGatekeepInfoCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	enabledStr := "disabled"
	if guildSettings.GatekeepEnabled {
		enabledStr = "enabled"
	}

	pendingRoleStr := "none"
	if guildSettings.GatekeepPendingRole != 0 {
		role, err := e.Client().Rest().GetRole(guildID, guildSettings.GatekeepPendingRole)
		if err == nil {
			pendingRoleStr = fmt.Sprintf("%s (%d)", role.Name, role.ID)
		} else {
			pendingRoleStr = fmt.Sprintf("ID: %d (could not lookup role)", guildSettings.GatekeepPendingRole)
		}
	}

	approvedRoleStr := "none"
	if guildSettings.GatekeepApprovedRole != 0 {
		role, err := e.Client().Rest().GetRole(guildID, guildSettings.GatekeepApprovedRole)
		if err == nil {
			approvedRoleStr = fmt.Sprintf("%s (%d)", role.Name, role.ID)
		} else {
			approvedRoleStr = fmt.Sprintf("ID: %d (could not lookup role)", guildSettings.GatekeepApprovedRole)
		}
	}

	addPendingRoleOnJoinStr := "no"
	if guildSettings.GatekeepAddPendingRoleOnJoin {
		addPendingRoleOnJoinStr = "yes"
	}

	approvalMessage := "(none)"
	if guildSettings.GatekeepApprovedMessage != "" {
		approvalMessage = guildSettings.GatekeepApprovedMessage
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf(
			"## Gatekeep\n"+
				"- Gatekeep is currently: **%s**\n"+
				"- Pending role: %s\n"+
				"- Approved role: %s\n"+
				"- Give pending role on join: %s\n\n"+
				"### Approval message: \n%s",
			enabledStr,
			pendingRoleStr,
			approvedRoleStr,
			addPendingRoleOnJoinStr,
			approvalMessage,
		)).
		Build())
}

func AdminGatekeepPendingRoleSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	role, hasRole := data.OptRole("role")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if hasRole {
		guildSettings.GatekeepPendingRole = role.ID
	} else {
		guildSettings.GatekeepPendingRole = 0
	}

	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set Gatekeep pending role. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	if !hasRole {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Gatekeep pending role cleared.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Gatekeep pending role set to %s.", role.Name)).
		Build())
}

func AdminGatekeepApprovedRoleSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	role, hasRole := data.OptRole("role")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if hasRole {
		guildSettings.GatekeepApprovedRole = role.ID
	} else {
		guildSettings.GatekeepApprovedRole = 0
	}

	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set Gatekeep approved role. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	if !hasRole {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Gatekeep approved role cleared.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Gatekeep approved role set to %s.", role.Name)).
		Build())
}

func AdminGatekeepAddPendingRoleOnJoinSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	enabled := data.Bool("enabled")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.GatekeepAddPendingRoleOnJoin = enabled
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set Gatekeep add pending role on join. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Gatekeep add pending role on join set to %t.", enabled).
		Build())
}

func AdminGatekeepApprovedMessageSetCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	message, hasMessage := data.OptString("message")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if hasMessage {
		guildSettings.GatekeepApprovedMessage = message
	} else {
		guildSettings.GatekeepApprovedMessage = ""
	}

	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set Gatekeep approved message. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	if !hasMessage {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Gatekeep approved message cleared.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Gatekeep approved message set to:\n%s", message).
		Build())

}

func AdminJoinLeaveInfoCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	joinEnabledStr := "disabled"
	joinMessage := "(none)"
	if guildSettings.JoinMessageEnabled {
		joinEnabledStr = "enabled"
		if guildSettings.JoinMessage != "" {
			joinMessage = guildSettings.JoinMessage
		}
	}

	leaveEnabledStr := "disabled"
	leaveMessage := "(none)"
	if guildSettings.LeaveMessageEnabled {
		leaveEnabledStr = "enabled"
		if guildSettings.LeaveMessage != "" {
			leaveMessage = guildSettings.LeaveMessage
		}
	}

	channel := "none"
	if guildSettings.JoinLeaveChannel != 0 {
		channel = fmt.Sprintf("<#%d>", guildSettings.JoinLeaveChannel)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf(
			"## Join/Leave Messages\n"+
				"- Join messages are currently: **%s**\n"+
				"- Leave messages are currently: **%s**\n"+
				"- Join/Leave channel: %s\n	"+
				"### Join message: \n%s\n"+
				"### Leave message: \n%s\n",
			joinEnabledStr,
			channel,
			leaveEnabledStr,
			joinMessage,
			leaveMessage,
		).Build())
}

func AdminJoinLeaveSetJoinEnabledCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	enabled := data.Bool("enabled")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.JoinMessageEnabled = enabled
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set join messages enabled. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Join messages enabled set to %t.", enabled)).
		Build())
}

func AdminJoinLeaveSetJoinMessageCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	message, hasMessage := data.OptString("message")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if !hasMessage {
		par := discord.NewParagraphTextInput("message", "Join message").
			WithPlaceholder("Join message, see `/admin join-leave help` for more info.").
			WithMinLength(1).
			WithValue(guildSettings.JoinMessage)

		return e.Modal(discord.NewModalCreateBuilder().
			SetTitle("Join Message").
			SetCustomID("/admin/join-leave/join-message/modal").
			SetContainerComponents(
				discord.NewActionRow(par)).
			Build(),
		)
	}

	guildSettings.JoinMessage = message
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set join message. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Join message set to:\n%s", message).
		Build())
}

func AdminJoinLeaveSetLeaveEnabledCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	enabled := data.Bool("enabled")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.LeaveMessageEnabled = enabled
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set leave messages enabled. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Leave messages enabled set to %t.", enabled)).
		Build())
}

func AdminJoinLeaveSetLeaveMessageCommandHandler(e *handler.CommandEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	data := e.SlashCommandInteractionData()
	message, hasMessage := data.OptString("message")

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	if !hasMessage {
		par := discord.NewParagraphTextInput("message", "Leave message").
			WithPlaceholder("Leave message, see `/admin join-leave help` for more info.").
			WithMinLength(1).
			WithValue(guildSettings.LeaveMessage)

		return e.Modal(discord.NewModalCreateBuilder().
			SetTitle("Leave Message").
			SetCustomID("/admin/join-leave/leave-message/modal").
			SetContainerComponents(
				discord.NewActionRow(par)).
			Build(),
		)
	}

	guildSettings.LeaveMessage = message
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set leave message. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Leave message set to:\n%s", message).
		Build())
}

func AdminJoinLeaveSetChannelCommandHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	channel := data.Channel("channel")

	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this command can only be used in a guild")
	}
	guildID := *guildIDRef

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.JoinLeaveChannel = channel.ID
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set join/leave channel. Please try again later.").
			Build())
		return fmt.Errorf("failed to set guild settings: %w", err)
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(fmt.Sprintf("Join/leave channel set to <#%d>.", channel.ID)).
		Build())
}

func AdminJoinLeaveJoinMessageModal(e *handler.ModalEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this modal can only be used in a guild")
	}

	guildID := *guildIDRef
	message, ok := e.Data.OptText("message")
	if !ok {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set join message. Please try again later.").
			Build())
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.JoinMessage = message
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		slog.Warn("Failed to set join message.", "err", err)
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set join message. Please try again later.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Join message set to:\n%s", message).
		Build())

}

func AdminJoinLeaveLeaveMessageModal(e *handler.ModalEvent) error {
	guildIDRef := e.GuildID()
	if guildIDRef == nil {
		return errors.New("this modal can only be used in a guild")
	}

	guildID := *guildIDRef
	message, ok := e.Data.OptText("message")
	if !ok {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set leave message. Please try again later.").
			Build())
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild settings: %w", err)
	}

	guildSettings.LeaveMessage = message
	err = model.SetGuildSettings(guildSettings)
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to set leave message. Please try again later.").
			Build())
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Leave message set to:\n%s", message).
		Build())
}

package commands

import (
	"fmt"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
)

var AdminCommand = discord.SlashCommandCreate{
	Name:                     "admin",
	Description:              "admin commands",
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionAdministrator),
	DMPermission:             utils.Ref(false),
	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "info",
			Description: "Show information about server configuration",
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "mod-channel",
			Description: "View or set the moderator channel",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionChannel{
					Name:         "channel",
					Description:  "The channel to set as the moderator channel",
					Required:     false,
					ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "infractions",
			Description: "View or set infraction-related settings",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionFloat{
					Name:        "half-life",
					Description: "The half-life of infractions in days (0 = no half-life)",
					Required:    false,
					MinValue:    utils.Ref(0.0),
					MaxValue:    utils.Ref(365.0),
				},
				discord.ApplicationCommandOptionBool{
					Name:        "notify-warned-user-join",
					Description: "Whether to notify moderator channel when warned user (re)joins the server",
					Required:    false,
				},
				discord.ApplicationCommandOptionFloat{
					Name:        "notify-threshold",
					Description: "The minimum severity of infractions to notify on (0 = always)",
					Required:    false,
					MinValue:    utils.Ref(0.0),
					MaxValue:    utils.Ref(100.0),
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "gatekeep",
			Description: "View or set gatekeep-related settings",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "enabled",
					Description: "Whether to enable the gatekeep system",
					Required:    false,
				},
				discord.ApplicationCommandOptionRole{
					Name:        "pending-role",
					Description: "The role to give to users pending approval",
					Required:    false,
				},
				discord.ApplicationCommandOptionRole{
					Name:        "approved-role",
					Description: "The role to give to approved users",
					Required:    false,
				},
				discord.ApplicationCommandOptionBool{
					Name:        "use-pending-role",
					Description: "Whether to give the pending role to users when they join",
					Required:    false,
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "gatekeep-message",
			Description: "Set the message to send to approved users",
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "join-leave",
			Description: "View or set join and leave message settings",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "join-enabled",
					Description: "Whether to enable join messages",
					Required:    false,
				},
				discord.ApplicationCommandOptionBool{
					Name:        "leave-enabled",
					Description: "Whether to enable leave messages",
					Required:    false,
				},
				discord.ApplicationCommandOptionChannel{
					Name:         "channel",
					Description:  "The channel to send join and leave messages",
					Required:     false,
					ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
				},
			},
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "join-message",
			Description: "Set the message to send when a user joins",
		},

		discord.ApplicationCommandOptionSubCommand{
			Name:        "leave-message",
			Description: "Set the message to send when a user leaves",
		},
	},
}

func AdminInfoHandler(e *handler.CommandEvent) error {
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	modChannel := modChannelInfo(settings)
	infractionSettings := infractionInfo(settings)
	gatekeepSettings := gatekeepInfo(settings)
	joinLeaveSettings := joinLeaveInfo(settings)

	message := fmt.Sprintf("# Server settings\n%s\n\n%s\n\n%s\n\n%s",
		modChannel, infractionSettings, gatekeepSettings, joinLeaveSettings)

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(message).
		SetEphemeral(true).
		AddActionRow(discord.NewPrimaryButton("Display for everyone", "/admin/show-all-button")).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminShowAllButtonHandler(e *handler.ComponentEvent) error {
	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(e.Message.Content).
		SetEmbeds(e.Message.Embeds...).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func modChannelInfo(settings *model.GuildSettings) string {
	modChannelInfo := "> This is the channel in which notifications and other information for moderators and administrators are sent."
	return fmt.Sprintf("**Moderator channel:** <#%d>\n%s",
		settings.ModeratorChannel, modChannelInfo)
}

func infractionInfo(settings *model.GuildSettings) string {
	infractionHalfLifeInfo := "> This is the half-life time of infractions' severity in days.\n> A half-life of 0 means that infractions never expire."
	infractionHalfLife := fmt.Sprintf("**Infraction half-life:** %.1f days\n%s",
		settings.InfractionHalfLifeDays, infractionHalfLifeInfo)

	notifyOnWarnedUserJoinInfo := "> This determines whether to notify the moderator channel when a warned user (re)joins the server."
	notifyOnWarnedUserJoin := fmt.Sprintf("**Notify on warned user join:** %s\n%s",
		utils.Iif(settings.NotifyOnWarnedUserJoin, "yes", "no"), notifyOnWarnedUserJoinInfo)

	notifyWarnSeverityThresholdInfo := "> This is the minimum severity of infractions to notify on.\n> A threshold of 0 means that all infractions are notified on."
	notifyWarnSeverityThreshold := fmt.Sprintf("**Notify warn severity threshold:** %.1f\n%s",
		settings.NotifyWarnSeverityThreshold, notifyWarnSeverityThresholdInfo)

	return fmt.Sprintf("## Infraction settings\n%s\n\n%s\n\n%s",
		infractionHalfLife, notifyOnWarnedUserJoin, notifyWarnSeverityThreshold)
}

func gatekeepInfo(settings *model.GuildSettings) string {
	gatekeepEnabledInfo := "> This determines whether to enable the gatekeep system."
	gatekeepEnabled := fmt.Sprintf("**Gatekeep enabled:** %s\n%s",
		utils.Iif(settings.GatekeepEnabled, "yes", "no"), gatekeepEnabledInfo)

	gatekeepPendingRoleInfo := "> This is the role given to users pending approval."
	gatekeepPendingRole := fmt.Sprintf("**Gatekeep pending role:** <@&%d>\n%s",
		settings.GatekeepPendingRole, gatekeepPendingRoleInfo)

	gatekeepApprovedRoleInfo := "> This is the role given to approved users."
	gatekeepApprovedRole := fmt.Sprintf("**Gatekeep approved role:** <@&%d>\n%s",
		settings.GatekeepApprovedRole, gatekeepApprovedRoleInfo)

	gatekeepAddPendingRoleOnJoinInfo := "> This determines whether to give the pending role to users when they join."
	gatekeepAddPendingRoleOnJoin := fmt.Sprintf("**Give pending role on join:** %s\n%s",
		utils.Iif(settings.GatekeepAddPendingRoleOnJoin, "yes", "no"), gatekeepAddPendingRoleOnJoinInfo)

	gatekeepApprovedMessageInfo := "Approved message can be viewed by using the `/admin gatekeep-message` command."

	return fmt.Sprintf("## Gatekeep settings\n%s\n\n%s\n\n%s\n\n%s\n\n*%s*",
		gatekeepEnabled, gatekeepPendingRole, gatekeepApprovedRole, gatekeepAddPendingRoleOnJoin, gatekeepApprovedMessageInfo)
}

func joinLeaveInfo(settings *model.GuildSettings) string {
	joinMessageEnabledInfo := "> This determines whether to enable join messages."
	joinMessageEnabled := fmt.Sprintf("**Join message enabled:** %s\n%s",
		utils.Iif(settings.JoinMessageEnabled, "yes", "no"), joinMessageEnabledInfo)

	leaveMessageEnabledInfo := "> This determines whether to enable leave messages."
	leaveMessageEnabled := fmt.Sprintf("**Leave message enabled:** %s\n%s",
		utils.Iif(settings.LeaveMessageEnabled, "yes", "no"), leaveMessageEnabledInfo)

	joinLeaveChannelInfo := "> This is the channel in which join and leave messages are sent."
	joinLeaveChannel := fmt.Sprintf("**Join/leave channel:** <#%d>\n%s",
		settings.JoinLeaveChannel, joinLeaveChannelInfo)

	joinLeaveMessageInfo := "The join/leave messages can be viewed by using the `/admin join-message` and `/admin leave-message` commands."

	return fmt.Sprintf("## Join/leave settings\n%s\n\n%s\n\n%s\n\n*%s*",
		joinMessageEnabled, leaveMessageEnabled, joinLeaveChannel, joinLeaveMessageInfo)
}

func AdminModChannelHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	channel, hasChannel := data.OptChannel("channel")
	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	if !hasChannel {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(modChannelInfo(settings)).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build())
	}

	settings.ModeratorChannel = channel.ID
	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContentf("Moderator channel set to <#%d>", channel.ID).
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminInfractionsHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	halfLife, hasHalfLife := data.OptFloat("half-life")
	if hasHalfLife {
		settings.InfractionHalfLifeDays = halfLife
		message += fmt.Sprintf("Infraction half-life set to %.1f days\n", halfLife)
	}

	notifyOnWarnedUserJoin, hasNotifyOnWarnedUserJoin := data.OptBool("notify-warned-user-join")
	if hasNotifyOnWarnedUserJoin {
		settings.NotifyOnWarnedUserJoin = notifyOnWarnedUserJoin
		message += fmt.Sprintf("Notify on warned user join set to %s\n", utils.Iif(notifyOnWarnedUserJoin, "yes", "no"))
	}

	notifyThreshold, hasNotifyThreshold := data.OptFloat("notify-threshold")
	if hasNotifyThreshold {
		settings.NotifyWarnSeverityThreshold = notifyThreshold
		message += fmt.Sprintf("Notify warn severity threshold set to %.1f\n", notifyThreshold)
	}

	if !utils.Any(hasHalfLife, hasNotifyThreshold, hasNotifyOnWarnedUserJoin) {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(infractionInfo(settings)).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build())
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(message).
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminGatekeepHandler(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	enabled, hasEnabled := data.OptBool("enabled")
	if hasEnabled {
		settings.GatekeepEnabled = enabled
		message += fmt.Sprintf("Gatekeep enabled set to %s\n", utils.Iif(enabled, "yes", "no"))
	}

	pendingRole, hasPendingRole := data.OptRole("pending-role")
	if hasPendingRole {
		settings.GatekeepPendingRole = pendingRole.ID
		message += fmt.Sprintf("Gatekeep pending role set to <@&%d>\n", pendingRole.ID)
	}

	approvedRole, hasApprovedRole := data.OptRole("approved-role")
	if hasApprovedRole {
		settings.GatekeepApprovedRole = approvedRole.ID
		message += fmt.Sprintf("Gatekeep approved role set to <@&%d>\n", approvedRole.ID)
	}

	usePendingRole, hasUsePendingRole := data.OptBool("use-pending-role")
	if hasUsePendingRole {
		settings.GatekeepAddPendingRoleOnJoin = usePendingRole
		message += fmt.Sprintf("Give pending role on join set to %s\n", utils.Iif(usePendingRole, "yes", "no"))
	}

	if !utils.Any(hasEnabled, hasPendingRole, hasApprovedRole, hasUsePendingRole) {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(gatekeepInfo(settings)).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build())
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(message).
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminGatekeepMessageHandler(e *handler.CommandEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Gatekeep approved message").
		SetDescription(settings.GatekeepApprovedMessage).
		Build()

	templateInfoEmbed := discord.NewEmbedBuilder().
		SetTitle("Placeholder values").
		SetDescription(utils.MessageTemplateInfo).
		Build()

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEmbeds(embed, templateInfoEmbed).
		AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/gatekeep-message/button")).
		SetAllowedMentions(&discord.AllowedMentions{}).
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func messageModal(customID, title, contents string) discord.ModalCreate {
	return discord.NewModalCreateBuilder().
		SetCustomID(customID).
		SetTitle(title).
		AddActionRow(discord.NewParagraphTextInput("message", title).WithValue(contents)).
		Build()
}

func AdminGatekeepMessageButtonHandler(e *handler.ComponentEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	return e.Modal(messageModal(
		"/admin/gatekeep-message/modal",
		"Gatekeep approved message",
		settings.GatekeepApprovedMessage,
	))
}

func AdminGatekeepMessageModalHandler(e *handler.ModalEvent) error {
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	message := e.Data.Text("message")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	_, err = mustache.RenderRaw(message, true, utils.MessageTemplateData{})
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContentf("The message contains data that is invalid; this may be caused by invalid placeholders.").
			SetEphemeral(true).
			Build())
	}

	settings.GatekeepApprovedMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent("Gatekeep approved message updated.").
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminJoinLeaveHandler(e *handler.CommandEvent) error {
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	message := ""

	joinEnabled, hasJoinEnabled := e.SlashCommandInteractionData().OptBool("join-enabled")
	if hasJoinEnabled {
		settings.JoinMessageEnabled = joinEnabled
		message += fmt.Sprintf("Join message enabled set to %s\n", utils.Iif(joinEnabled, "yes", "no"))
	}

	leaveEnabled, hasLeaveEnabled := e.SlashCommandInteractionData().OptBool("leave-enabled")
	if hasLeaveEnabled {
		settings.LeaveMessageEnabled = leaveEnabled
		message += fmt.Sprintf("Leave message enabled set to %s\n", utils.Iif(leaveEnabled, "yes", "no"))
	}

	channel, hasChannel := e.SlashCommandInteractionData().OptChannel("channel")
	if hasChannel {
		settings.JoinLeaveChannel = channel.ID
		message += fmt.Sprintf("Join/leave channel set to <#%d>\n", channel.ID)
	}

	if !utils.Any(hasJoinEnabled, hasLeaveEnabled, hasChannel) {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(joinLeaveInfo(settings)).
			SetEphemeral(true).
			SetAllowedMentions(&discord.AllowedMentions{}).
			Build())
	}

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent(message).
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminJoinMessageHandler(e *handler.CommandEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Join message").
		SetDescription(settings.JoinMessage).
		Build()

	templateInfoEmbed := discord.NewEmbedBuilder().
		SetTitle("Placeholder values").
		SetDescription(utils.MessageTemplateInfo).
		Build()

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEmbeds(embed, templateInfoEmbed).
		AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/join-message/button")).
		SetAllowedMentions(&discord.AllowedMentions{}).
		SetEphemeral(true).
		Build())
}

func AdminJoinMessageButtonHandler(e *handler.ComponentEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	return e.Modal(messageModal(
		"/admin/join-message/modal",
		"Join message",
		settings.JoinMessage,
	))
}

func AdminJoinMessageModalHandler(e *handler.ModalEvent) error {
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	message := e.Data.Text("message")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	_, err = mustache.RenderRaw(message, true, utils.MessageTemplateData{})
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContentf("The message contains data that is invalid; this may be caused by invalid placeholders.").
			SetEphemeral(true).
			Build())
	}

	settings.JoinMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent("Join message updated.").
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

func AdminLeaveMessageHandler(e *handler.CommandEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	embed := discord.NewEmbedBuilder().
		SetTitle("Leave message").
		SetDescription(settings.LeaveMessage).
		Build()

	templateInfoEmbed := discord.NewEmbedBuilder().
		SetTitle("Placeholder values").
		SetDescription(utils.MessageTemplateInfo).
		Build()

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetEmbeds(embed, templateInfoEmbed).
		AddActionRow(discord.NewPrimaryButton("Edit message", "/admin/leave-message/button")).
		SetAllowedMentions(&discord.AllowedMentions{}).
		SetEphemeral(true).
		Build())
}

func AdminLeaveMessageButtonHandler(e *handler.ComponentEvent) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return ErrEventNoGuildID
	}

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	return e.Modal(messageModal(
		"/admin/leave-message/modal",
		"Leave message",
		settings.LeaveMessage,
	))
}

func AdminLeaveMessageModalHandler(e *handler.ModalEvent) error {
	guild, inGuild := e.Guild()
	if !inGuild {
		return nil
	}
	message := e.Data.Text("message")

	settings, err := model.GetGuildSettings(guild.ID)
	if err != nil {
		return err
	}

	_, err = mustache.RenderRaw(message, true, utils.MessageTemplateData{})
	if err != nil {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetContentf("The message contains data that is invalid; this may be caused by invalid placeholders.").
			SetEphemeral(true).
			Build())
	}

	settings.LeaveMessage = message

	err = model.SetGuildSettings(settings)
	if err != nil {
		return err
	}

	return e.CreateMessage(discord.NewMessageCreateBuilder().
		SetContent("Leave message updated.").
		SetEphemeral(true).
		SetAllowedMentions(&discord.AllowedMentions{}).
		Build())
}

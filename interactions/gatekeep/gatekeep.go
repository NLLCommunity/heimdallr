package gatekeep

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
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

	restGuild, err := e.Client().Rest.GetGuild(*e.GuildID(), false)
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
			),
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
		return e.CreateMessage(interactions.EphemeralMessageContent("Failed to get guild information."))
	}

	if !guildSettings.GatekeepEnabled {
		return e.CreateMessage(interactions.EphemeralMessageContent("Gatekeep is not enabled in this server."))
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
			),
		)
	}

	if guildSettings.GatekeepApprovedRole != 0 {
		err = e.Client().Rest.AddMemberRole(
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
		err = e.Client().Rest.RemoveMemberRole(
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

	hasV2 := guildSettings.GatekeepApprovedMessageV2 && guildSettings.GatekeepApprovedMessageV2Json != ""
	hasPlain := guildSettings.GatekeepApprovedMessage != ""

	if !hasV2 && !hasPlain {
		slog.Info("No approved message set; not sending message.")
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"No approved message set; not sending message. Roles have been set.",
			),
		)
	}

	channel, channelOk := resolveApprovedMessageChannel(guildSettings, guild)
	if !channelOk {
		slog.Warn(
			"No channel configured for approved message; skipping send.",
			"guild_id", guild.ID,
			"user_id", member.User.ID,
		)
		return e.CreateMessage(
			interactions.EphemeralMessageContent(
				"User approved, but no channel is configured for the welcome message. Set a Join/Leave channel in settings (or a system channel for this server).",
			),
		)
	}

	templateData := utils.NewMessageTemplateData(member.Member, guild)

	data := &gatekeepData{
		approver:      e.User(),
		client:        e.Client(),
		guild:         guild,
		member:        member,
		guildSettings: guildSettings,
		templateData:  templateData,
		channel:       channel,
	}

	if hasV2 {
		_, err = createV2ApprovedMessage(data)
	} else {
		_, err = createV1ApprovedMessage(data)
	}
	if err != nil {
		_, err = e.CreateFollowupMessage(
			interactions.EphemeralMessageContent(
				"Failed to send message to approved user.",
			),
		)
		return err
	}

	_, err = e.CreateFollowupMessage(
		interactions.EphemeralMessageContent(
			"User has been approved!",
		),
	)
	return err
}

type gatekeepData struct {
	approver      discord.User
	client        *bot.Client
	guild         discord.Guild
	member        discord.ResolvedMember
	guildSettings *model.GuildSettings
	templateData  utils.MessageTemplateData
	channel       snowflake.ID
}

// resolveApprovedMessageChannel picks the channel to send the approval
// message in: the configured Join/Leave channel, falling back to the guild's
// system channel. Returns (0, false) if neither is set so callers can surface
// a clear error instead of failing on CreateMessage with channel ID 0.
func resolveApprovedMessageChannel(settings *model.GuildSettings, guild discord.Guild) (snowflake.ID, bool) {
	if settings.JoinLeaveChannel != 0 {
		return settings.JoinLeaveChannel, true
	}
	if guild.SystemChannelID != nil {
		return *guild.SystemChannelID, true
	}
	return 0, false
}

func createV1ApprovedMessage(
	data *gatekeepData,
) (m *discord.Message, err error) {
	contents, renderErr := mustache.RenderRaw(data.guildSettings.GatekeepApprovedMessage, true, data.templateData)
	if renderErr != nil {
		slog.Warn(
			"Failed to render approved message template.",
			"guild_id", data.guild.ID,
			"user_id", data.member.User.ID,
			"err", renderErr,
		)
		return nil, renderErr
	}

	return data.client.Rest.CreateMessage(
		data.channel,
		discord.NewMessageCreate().
			WithContent(
				contents+
					fmt.Sprintf("\n\n-# Approved by %s", data.approver.Mention()),
			).
			WithAllowedMentions(
				&discord.AllowedMentions{
					Users: []snowflake.ID{data.member.User.ID},
				},
			),
	)
}

func createV2ApprovedMessage(
	data *gatekeepData,
) (m *discord.Message, err error) {
	emojiMap := make(map[string]discord.Emoji)
	for emoji := range data.client.Caches.Emojis(data.guild.ID) {
		emojiMap[strings.ToLower(emoji.Name)] = emoji
	}

	components, compErr := utils.BuildV2Message(data.guildSettings.GatekeepApprovedMessageV2Json, data.templateData, emojiMap)
	if compErr != nil {
		slog.Warn("Failed to build V2 approved message.", "err", compErr)
		return nil, compErr
	}

	components = append(components, discord.NewTextDisplay(
		fmt.Sprintf("-# Approved by %s", data.approver.Mention()),
	))

	return data.client.Rest.CreateMessage(data.channel, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: components,
		AllowedMentions: &discord.AllowedMentions{
			Users: []snowflake.ID{data.member.User.ID},
		},
	})
}

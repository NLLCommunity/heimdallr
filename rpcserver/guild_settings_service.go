package rpcserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	heimdallrv1 "github.com/NLLCommunity/heimdallr/gen/heimdallr/v1"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

type guildSettingsService struct {
	client *bot.Client
}

// isGuildAdmin checks whether a user has admin permission in a guild.
// It checks guild ownership first, then tries the member cache, then falls
// back to the REST API.
func isGuildAdmin(client *bot.Client, guild discord.Guild, userID snowflake.ID) bool {
	if guild.OwnerID == userID {
		return true
	}

	member, ok := client.Caches.Member(guild.ID, userID)
	if !ok {
		m, err := client.Rest.GetMember(guild.ID, userID)
		if err != nil {
			return false
		}
		member = *m
	}

	perms := client.Caches.MemberPermissions(member)
	return perms.Has(discord.PermissionAdministrator)
}

// checkGuildAdmin verifies the session user has admin permission in the given guild.
func checkGuildAdmin(ctx context.Context, client *bot.Client, guildIDStr string) (snowflake.ID, error) {
	session := SessionFromContext(ctx)
	if session == nil {
		return 0, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	guildID, err := snowflake.Parse(guildIDStr)
	if err != nil {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid guild ID"))
	}

	guild, ok := client.Caches.Guild(guildID)
	if !ok {
		return 0, connect.NewError(connect.CodePermissionDenied, errors.New("bot is not in this guild"))
	}

	if !isGuildAdmin(client, guild, session.UserID) {
		return 0, connect.NewError(connect.CodePermissionDenied, errors.New("you do not have administrator permission in this guild"))
	}

	return guildID, nil
}

func idStr(id snowflake.ID) string {
	if id == 0 {
		return ""
	}
	return id.String()
}

func parseSnowflake(s string) snowflake.ID {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return snowflake.ID(v)
}

// --- ModChannel ---

func (s *guildSettingsService) GetModChannel(ctx context.Context, req *heimdallrv1.GetModChannelRequest) (*heimdallrv1.ModChannelSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	return &heimdallrv1.ModChannelSettings{
		GuildId:          guildID.String(),
		ModeratorChannel: idStr(settings.ModeratorChannel),
	}, nil
}

func (s *guildSettingsService) UpdateModChannel(ctx context.Context, req *heimdallrv1.UpdateModChannelRequest) (*heimdallrv1.ModChannelSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	settings.ModeratorChannel = parseSnowflake(proto.GetModeratorChannel())

	if err := model.SetGuildSettings(settings); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save settings"))
	}

	return &heimdallrv1.ModChannelSettings{
		GuildId:          guildID.String(),
		ModeratorChannel: idStr(settings.ModeratorChannel),
	}, nil
}

// --- InfractionSettings ---

func (s *guildSettingsService) GetInfractionSettings(ctx context.Context, req *heimdallrv1.GetInfractionSettingsRequest) (*heimdallrv1.InfractionSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	return &heimdallrv1.InfractionSettings{
		GuildId:                     guildID.String(),
		HalfLifeDays:                settings.InfractionHalfLifeDays,
		NotifyOnWarnedUserJoin:      settings.NotifyOnWarnedUserJoin,
		NotifyWarnSeverityThreshold: settings.NotifyWarnSeverityThreshold,
	}, nil
}

func (s *guildSettingsService) UpdateInfractionSettings(ctx context.Context, req *heimdallrv1.UpdateInfractionSettingsRequest) (*heimdallrv1.InfractionSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	settings.InfractionHalfLifeDays = proto.GetHalfLifeDays()
	settings.NotifyOnWarnedUserJoin = proto.GetNotifyOnWarnedUserJoin()
	settings.NotifyWarnSeverityThreshold = proto.GetNotifyWarnSeverityThreshold()

	if err := model.SetGuildSettings(settings); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save settings"))
	}

	return &heimdallrv1.InfractionSettings{
		GuildId:                     guildID.String(),
		HalfLifeDays:                settings.InfractionHalfLifeDays,
		NotifyOnWarnedUserJoin:      settings.NotifyOnWarnedUserJoin,
		NotifyWarnSeverityThreshold: settings.NotifyWarnSeverityThreshold,
	}, nil
}

// --- GatekeepSettings ---

func (s *guildSettingsService) GetGatekeepSettings(ctx context.Context, req *heimdallrv1.GetGatekeepSettingsRequest) (*heimdallrv1.GatekeepSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	return &heimdallrv1.GatekeepSettings{
		GuildId:                guildID.String(),
		Enabled:                settings.GatekeepEnabled,
		PendingRole:            idStr(settings.GatekeepPendingRole),
		ApprovedRole:           idStr(settings.GatekeepApprovedRole),
		AddPendingRoleOnJoin:   settings.GatekeepAddPendingRoleOnJoin,
		ApprovedMessage:        settings.GatekeepApprovedMessage,
		ApprovedMessageV2:      settings.GatekeepApprovedMessageV2,
		ApprovedMessageV2Json:  settings.GatekeepApprovedMessageV2Json,
	}, nil
}

func (s *guildSettingsService) UpdateGatekeepSettings(ctx context.Context, req *heimdallrv1.UpdateGatekeepSettingsRequest) (*heimdallrv1.GatekeepSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	settings.GatekeepEnabled = proto.GetEnabled()
	settings.GatekeepPendingRole = parseSnowflake(proto.GetPendingRole())
	settings.GatekeepApprovedRole = parseSnowflake(proto.GetApprovedRole())
	settings.GatekeepAddPendingRoleOnJoin = proto.GetAddPendingRoleOnJoin()
	settings.GatekeepApprovedMessage = proto.GetApprovedMessage()
	settings.GatekeepApprovedMessageV2 = proto.GetApprovedMessageV2()
	settings.GatekeepApprovedMessageV2Json = proto.GetApprovedMessageV2Json()

	if err := model.SetGuildSettings(settings); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save settings"))
	}

	return &heimdallrv1.GatekeepSettings{
		GuildId:                guildID.String(),
		Enabled:                settings.GatekeepEnabled,
		PendingRole:            idStr(settings.GatekeepPendingRole),
		ApprovedRole:           idStr(settings.GatekeepApprovedRole),
		AddPendingRoleOnJoin:   settings.GatekeepAddPendingRoleOnJoin,
		ApprovedMessage:        settings.GatekeepApprovedMessage,
		ApprovedMessageV2:      settings.GatekeepApprovedMessageV2,
		ApprovedMessageV2Json:  settings.GatekeepApprovedMessageV2Json,
	}, nil
}

// --- JoinLeaveSettings ---

func (s *guildSettingsService) GetJoinLeaveSettings(ctx context.Context, req *heimdallrv1.GetJoinLeaveSettingsRequest) (*heimdallrv1.JoinLeaveSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	return &heimdallrv1.JoinLeaveSettings{
		GuildId:             guildID.String(),
		JoinMessageEnabled:  settings.JoinMessageEnabled,
		JoinMessage:         settings.JoinMessage,
		JoinMessageV2:       settings.JoinMessageV2,
		JoinMessageV2Json:   settings.JoinMessageV2Json,
		LeaveMessageEnabled: settings.LeaveMessageEnabled,
		LeaveMessage:        settings.LeaveMessage,
		LeaveMessageV2:      settings.LeaveMessageV2,
		LeaveMessageV2Json:  settings.LeaveMessageV2Json,
		Channel:             idStr(settings.JoinLeaveChannel),
	}, nil
}

func (s *guildSettingsService) UpdateJoinLeaveSettings(ctx context.Context, req *heimdallrv1.UpdateJoinLeaveSettingsRequest) (*heimdallrv1.JoinLeaveSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	settings.JoinMessageEnabled = proto.GetJoinMessageEnabled()
	settings.JoinMessage = proto.GetJoinMessage()
	settings.JoinMessageV2 = proto.GetJoinMessageV2()
	settings.JoinMessageV2Json = proto.GetJoinMessageV2Json()
	settings.LeaveMessageEnabled = proto.GetLeaveMessageEnabled()
	settings.LeaveMessage = proto.GetLeaveMessage()
	settings.LeaveMessageV2 = proto.GetLeaveMessageV2()
	settings.LeaveMessageV2Json = proto.GetLeaveMessageV2Json()
	settings.JoinLeaveChannel = parseSnowflake(proto.GetChannel())

	if err := model.SetGuildSettings(settings); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save settings"))
	}

	return &heimdallrv1.JoinLeaveSettings{
		GuildId:             guildID.String(),
		JoinMessageEnabled:  settings.JoinMessageEnabled,
		JoinMessage:         settings.JoinMessage,
		JoinMessageV2:       settings.JoinMessageV2,
		JoinMessageV2Json:   settings.JoinMessageV2Json,
		LeaveMessageEnabled: settings.LeaveMessageEnabled,
		LeaveMessage:        settings.LeaveMessage,
		LeaveMessageV2:      settings.LeaveMessageV2,
		LeaveMessageV2Json:  settings.LeaveMessageV2Json,
		Channel:             idStr(settings.JoinLeaveChannel),
	}, nil
}

// --- AntiSpamSettings ---

func (s *guildSettingsService) GetAntiSpamSettings(ctx context.Context, req *heimdallrv1.GetAntiSpamSettingsRequest) (*heimdallrv1.AntiSpamSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	return &heimdallrv1.AntiSpamSettings{
		GuildId:         guildID.String(),
		Enabled:         settings.AntiSpamEnabled,
		Count:           int32(settings.AntiSpamCount),
		CooldownSeconds: int32(settings.AntiSpamCooldownSeconds),
	}, nil
}

func (s *guildSettingsService) UpdateAntiSpamSettings(ctx context.Context, req *heimdallrv1.UpdateAntiSpamSettingsRequest) (*heimdallrv1.AntiSpamSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	settings.AntiSpamEnabled = proto.GetEnabled()
	settings.AntiSpamCount = int(proto.GetCount())
	settings.AntiSpamCooldownSeconds = int(proto.GetCooldownSeconds())

	if err := model.SetGuildSettings(settings); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save settings"))
	}

	return &heimdallrv1.AntiSpamSettings{
		GuildId:         guildID.String(),
		Enabled:         settings.AntiSpamEnabled,
		Count:           int32(settings.AntiSpamCount),
		CooldownSeconds: int32(settings.AntiSpamCooldownSeconds),
	}, nil
}

// --- BanFooterSettings ---

func (s *guildSettingsService) GetBanFooterSettings(ctx context.Context, req *heimdallrv1.GetBanFooterSettingsRequest) (*heimdallrv1.BanFooterSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	return &heimdallrv1.BanFooterSettings{
		GuildId:    guildID.String(),
		Footer:     settings.BanFooter,
		AlwaysSend: settings.AlwaysSendBanFooter,
	}, nil
}

func (s *guildSettingsService) UpdateBanFooterSettings(ctx context.Context, req *heimdallrv1.UpdateBanFooterSettingsRequest) (*heimdallrv1.BanFooterSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load settings"))
	}

	settings.BanFooter = proto.GetFooter()
	settings.AlwaysSendBanFooter = proto.GetAlwaysSend()

	if err := model.SetGuildSettings(settings); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save settings"))
	}

	return &heimdallrv1.BanFooterSettings{
		GuildId:    guildID.String(),
		Footer:     settings.BanFooter,
		AlwaysSend: settings.AlwaysSendBanFooter,
	}, nil
}

// --- ModmailSettings ---

func (s *guildSettingsService) GetModmailSettings(ctx context.Context, req *heimdallrv1.GetModmailSettingsRequest) (*heimdallrv1.ModmailSettings, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	ms, err := model.GetModmailSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load modmail settings"))
	}

	return &heimdallrv1.ModmailSettings{
		GuildId:                   guildID.String(),
		ReportThreadsChannel:      idStr(ms.ReportThreadsChannel),
		ReportNotificationChannel: idStr(ms.ReportNotificationChannel),
		ReportPingRole:            idStr(ms.ReportPingRole),
	}, nil
}

func (s *guildSettingsService) UpdateModmailSettings(ctx context.Context, req *heimdallrv1.UpdateModmailSettingsRequest) (*heimdallrv1.ModmailSettings, error) {
	proto := req.GetSettings()
	guildID, err := checkGuildAdmin(ctx, s.client, proto.GetGuildId())
	if err != nil {
		return nil, err
	}

	ms, err := model.GetModmailSettings(guildID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load modmail settings"))
	}

	ms.ReportThreadsChannel = parseSnowflake(proto.GetReportThreadsChannel())
	ms.ReportNotificationChannel = parseSnowflake(proto.GetReportNotificationChannel())
	ms.ReportPingRole = parseSnowflake(proto.GetReportPingRole())

	if err := model.SetModmailSettings(ms); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save modmail settings"))
	}

	return &heimdallrv1.ModmailSettings{
		GuildId:                   guildID.String(),
		ReportThreadsChannel:      idStr(ms.ReportThreadsChannel),
		ReportNotificationChannel: idStr(ms.ReportNotificationChannel),
		ReportPingRole:            idStr(ms.ReportPingRole),
	}, nil
}

// --- Guild Data (Channels & Roles) ---

func (s *guildSettingsService) ListChannels(ctx context.Context, req *heimdallrv1.ListChannelsRequest) (*heimdallrv1.ListChannelsResponse, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	var channels []*heimdallrv1.Channel
	for ch := range s.client.Caches.ChannelsForGuild(guildID) {
		var parentID string
		if pid := ch.ParentID(); pid != nil {
			parentID = pid.String()
		}
		channels = append(channels, &heimdallrv1.Channel{
			Id:       ch.ID().String(),
			Name:     ch.Name(),
			Type:     int32(ch.Type()),
			Position: int32(ch.Position()),
			ParentId: parentID,
		})
	}

	return &heimdallrv1.ListChannelsResponse{Channels: channels}, nil
}

func (s *guildSettingsService) ListRoles(ctx context.Context, req *heimdallrv1.ListRolesRequest) (*heimdallrv1.ListRolesResponse, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	var roles []*heimdallrv1.Role
	for role := range s.client.Caches.Roles(guildID) {
		roles = append(roles, &heimdallrv1.Role{
			Id:       role.ID.String(),
			Name:     role.Name,
			Color:    int32(role.Color),
			Position: int32(role.Position),
			Managed:  role.Managed,
		})
	}

	return &heimdallrv1.ListRolesResponse{Roles: roles}, nil
}

func (s *guildSettingsService) GetTemplatePlaceholders(ctx context.Context, _ *heimdallrv1.GetTemplatePlaceholdersRequest) (*heimdallrv1.GetTemplatePlaceholdersResponse, error) {
	if SessionFromContext(ctx) == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	placeholders := make([]*heimdallrv1.TemplatePlaceholder, len(utils.MessageTemplatePlaceholders))
	for i, p := range utils.MessageTemplatePlaceholders {
		placeholders[i] = &heimdallrv1.TemplatePlaceholder{
			Placeholder: p.Placeholder,
			Description: p.Description,
		}
	}
	return &heimdallrv1.GetTemplatePlaceholdersResponse{Placeholders: placeholders}, nil
}

// buildEmojiMap builds a lowercase emoji name → Emoji lookup from the guild cache.
func (s *guildSettingsService) buildEmojiMap(guildID snowflake.ID) map[string]discord.Emoji {
	emojiMap := make(map[string]discord.Emoji)
	for emoji := range s.client.Caches.Emojis(guildID) {
		emojiMap[strings.ToLower(emoji.Name)] = emoji
	}
	return emojiMap
}

// --- SendComponentsMessage ---

func (s *guildSettingsService) SendComponentsMessage(ctx context.Context, req *heimdallrv1.SendComponentsMessageRequest) (*heimdallrv1.SendComponentsMessageResponse, error) {
	guildID, err := checkGuildAdmin(ctx, s.client, req.GetGuildId())
	if err != nil {
		return nil, err
	}

	channelID, err := snowflake.Parse(req.GetChannelId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid channel ID"))
	}

	ch, ok := s.client.Caches.GuildMessageChannel(channelID)
	if !ok || ch.GuildID() != guildID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("channel not found in this guild"))
	}

	var parsed any
	if err := json.Unmarshal([]byte(req.GetComponentsJson()), &parsed); err != nil {
		slog.Error("invalid components JSON from client", "error", err)
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid components JSON"))
	}

	utils.ResolveEmojis(parsed, s.buildEmojiMap(guildID))

	resolvedJSON, err := json.Marshal(parsed)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to re-encode components"))
	}

	components, err := utils.ParseComponents(string(resolvedJSON))
	if err != nil {
		slog.Error("failed to parse resolved components", "error", err)
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid components JSON"))
	}

	msg, err := s.client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: components,
	})
	if err != nil {
		slog.Error("failed to send Discord message", "error", err, "channel_id", channelID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to send message"))
	}

	return &heimdallrv1.SendComponentsMessageResponse{
		MessageId: msg.ID.String(),
	}, nil
}

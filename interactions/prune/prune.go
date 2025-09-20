package prune

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/prune-pending-members", PruneHandler)
	r.Component("/button/prune-members/confirm/{pruneID}", PruneConfirmHandler)
	r.Component("/button/prune-members/cancel/{pruneID}", PruneCancelHandler)

	return []discord.ApplicationCommandCreate{PruneCommand}
}

var PruneCommand = discord.SlashCommandCreate{
	Name: "prune-pending-members",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "fjern-ventende-medlemmer",
	},
	Description: "Prune members.",
	DescriptionLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "Fjern medlemmer.",
	},

	Contexts: []discord.InteractionContextType{
		discord.InteractionContextTypeGuild,
	},
	IntegrationTypes: []discord.ApplicationIntegrationType{
		discord.ApplicationIntegrationTypeGuildInstall,
	},

	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionManageGuild),

	Options: []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionInt{
			Name: "days",
			NameLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "dager",
			},
			Description: "The number of days to prune members for.",
			DescriptionLocalizations: map[discord.Locale]string{
				discord.LocaleNorwegian: "Antall dager å fjerne medlemmer for.",
			},
			Required: true,

			MinValue: utils.Ref(0),
			MaxValue: utils.Ref(90),
		},
	},
}

func PruneConfirmHandler(e *handler.ComponentEvent) error {
	if e.GuildID() == nil {
		return ix.ErrEventNoGuildID
	}
	guildID := *e.GuildID()

	pruneIDString, ok := e.Vars["pruneID"]
	if !ok {
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred.").Build())
	}

	pruneID, err := uuid.Parse(pruneIDString)
	if err != nil {
		slog.Warn(
			"failed to parse prune ID",
			"guild_id", guildID,
			"prune_id", pruneIDString,
		)
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred.").Build())
	}

	messages := kickMembers(e.Client(), guildID, pruneID)

	err = removeKickedMembersAndNotify(e, guildID, pruneID, messages)

	return err
}

func removeKickedMembersAndNotify(e *handler.ComponentEvent, guildID snowflake.ID, pruneID uuid.UUID, messages string) (
	err error,
) {
	members, err := model.GetPrunedMembers(pruneID, guildID)
	if err != nil {
		err = fmt.Errorf("failed to retrieve pruned members: %w", err)
		return
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		err = fmt.Errorf("failed to retrieve guild settings: %w", err)
		return
	}
	modChannelMessages := make([]string, len(members))
	joinleaveMessages := make([]string, len(members))

	for i, member := range members {
		modChannelMessages[i] = fmt.Sprintf("-# %s", getUsernameOrID(e.Client(), guildID, member.UserID))
		text, err := renderLeaveMessage(e.Client(), guildID, member.UserID)
		if err == nil {
			joinleaveMessages[i] = text
		} else {
			joinleaveMessages[i] = fmt.Sprintf(
				"-# `%s` (ID: `%s`) left the server.",
				getUsernameOrID(e.Client(), guildID, member.UserID),
				member.UserID,
			)
		}
	}

	moderatorMessage := discord.NewMessageCreateBuilder().
		SetContentf(
			"## The following users have been pruned:\n%s\n\n%s",
			strings.Join(modChannelMessages, "\n"),
			utils.Iif(messages == "", "", "### Messages:\n"+messages),
		)
	joinleaveMessage := discord.NewMessageCreateBuilder().
		SetContent(strings.Join(joinleaveMessages, "\n"))

	if settings.ModeratorChannel != 0 {
		err := e.UpdateMessage(
			discord.NewMessageUpdateBuilder().
				SetContent("Users have been pruned").SetContainerComponents().Build(),
		)
		if err != nil {
			slog.Warn(
				"failed to create prune message",
				"guild_id", guildID,
				"channel_id", settings.ModeratorChannel,
				"err", err,
			)
		}

		_, err = e.Client().Rest().CreateMessage(settings.ModeratorChannel, moderatorMessage.Build())
		if err != nil {
			slog.Warn("failed to create prune interaction response", "guild_id", guildID, "err", err)
		}
	} else {
		err := e.CreateMessage(moderatorMessage.SetEphemeral(true).Build())
		if err != nil {
			slog.Warn("failed to create prune interaction response", "guild_id", guildID, "err", err)
		}
	}

	if settings.JoinLeaveChannel != 0 && settings.LeaveMessageEnabled {
		_, err := e.Client().Rest().CreateMessage(settings.JoinLeaveChannel, joinleaveMessage.Build())
		if err != nil {
			slog.Error(
				"failed to send leave message for pruned users",
				"guild_id", guildID,
				"channel_id", settings.JoinLeaveChannel,
				"err", err,
			)
		}
	}

	err = model.RemovePrunedMembers(guildID)
	if err != nil {
		slog.Error("failed to remove pruned members from prune table", "err", err)
	}

	err = model.RemoveMembersByPruneID(pruneID, guildID)
	if err != nil {
		slog.Error("failed to remove members from prune table", "err", err)
	}

	return
}

func kickMembers(client bot.Client, guildID snowflake.ID, pruneID uuid.UUID) (messages string) {
	members, err := model.GetMembersToPrune(pruneID, guildID)
	if err != nil {
		slog.Warn(
			"failed to get members to prune.",
			"guild_id", guildID,
			"prune_id", pruneID,
			"err", err,
		)
	}

	for _, member := range members {
		err := model.SetMemberPruned(guildID, member.UserID, true)
		if err != nil {
			messages += fmt.Sprintf("Failed to kick %s", getUsernameOrID(client, guildID, member.UserID))
			slog.Warn(
				"failed to set user to pruned",
				"guild_id", guildID,
				"user_id", member.UserID,
			)
			continue
		}

		err = client.Rest().RemoveMember(guildID, member.UserID)
		if err != nil {
			_ = model.SetMemberPruned(guildID, member.UserID, false)
			slog.Warn(
				"failed to prune/kick member",
				"guild_id", guildID,
				"user_id", member.UserID,
			)
			messages += fmt.Sprintf("Failed to kick %s", getUsernameOrID(client, guildID, member.UserID))
		}
	}

	return messages
}

func PruneCancelHandler(e *handler.ComponentEvent) error {
	if e.GuildID() == nil {
		return ix.ErrEventNoGuildID
	}
	pruneIDString, ok := e.Vars["pruneID"]
	if !ok {
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred.").Build())
	}

	pruneID, err := uuid.Parse(pruneIDString)
	if err != nil {
		slog.Warn(
			"failed to parse prune ID",
			"guild_id", *e.GuildID(),
			"prune_id", pruneIDString,
		)
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred.").Build())
	}

	err = model.RemoveMembersByPruneID(pruneID, *e.GuildID())
	if err != nil {
		slog.Warn(
			"failed to discard members pending prune",
			"guild_id", *e.GuildID(),
			"pruneID", pruneID,
		)
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to discard prune list").Build())
	}

	return e.UpdateMessage(
		discord.NewMessageUpdateBuilder().
			SetContent(e.Message.Content + "\n\n**Cancelled!**").SetContainerComponents().Build(),
	)
}

func PruneHandler(e *handler.CommandEvent) error {
	if e.GuildID() == nil {
		return ix.ErrEventNoGuildID
	}
	days := e.SlashCommandInteractionData().Int("days")

	guildSettings, err := model.GetGuildSettings(*e.GuildID())
	if err != nil {
		_ = e.CreateMessage(
			ix.EphemeralMessageContent(
				"Failed to prune members: could not get guild settings.",
			).Build(),
		)
		return err
	}

	if guildSettings.GatekeepPendingRole == 0 {
		return e.CreateMessage(
			ix.EphemeralMessageContent(
				"Failed to prune members: no pending role set. This command will only prune pending members.",
			).Build(),
		)
	}

	_ = e.DeferCreateMessage(true)
	prunableMembers, err := getPrunableMembers(e, days, guildSettings)
	if err != nil {
		_, err = e.CreateFollowupMessage(
			ix.EphemeralMessageContent(
				"Failed to prune members: could not get member list.",
			).Build(),
		)
		return err
	}

	pruneID := uuid.New()
	message, err := preparePruneMembers(pruneID, prunableMembers)
	if err != nil {
		slog.Error("Failed to prune members.", "err", err)
		_, err = e.CreateFollowupMessage(ix.EphemeralMessageContent("Failed to prune members: could not process list.").Build())
		return err
	}

	_, err = e.CreateFollowupMessage(message)

	return err
}

func preparePruneMembers(pruneID uuid.UUID, members []discord.Member) (
	discord.MessageCreate, error,
) {
	err := model.AddMembersToBePruned(pruneID, members)
	if err != nil {
		return discord.MessageCreate{}, err
	}

	content := fmt.Sprintf("## The following %d members will be pruned and kicked from the server\n", len(members))
	for _, member := range members {
		content += fmt.Sprintf("- `%s` (`%s`)\n", member.User.Username, member.User.ID)
	}

	message := ix.EphemeralMessageContent(content).
		AddActionRow(
			discord.NewDangerButton("Prune members", fmt.Sprintf("/button/prune-members/confirm/%s", pruneID)),
			discord.NewSecondaryButton("Cancel", fmt.Sprintf("/button/prune-members/cancel/%s", pruneID)),
		).Build()

	return message, nil
}

func getPrunableMembers(
	e *handler.CommandEvent, days int, guildSettings *model.GuildSettings,
) (members []discord.Member, err error) {
	maxTimeDiff := time.Duration(days) * time.Hour * 24

	for member := range utils.GetMembersIter(e.Client().Rest(), *e.GuildID()) {
		if member.Error != nil {
			return nil, member.Error
		}
		member := member.Value

		if !utils.HasRole(member, guildSettings.GatekeepPendingRole) {
			continue
		}

		if utils.HasRole(member, guildSettings.GatekeepApprovedRole) {
			continue
		}

		if time.Since(member.JoinedAt) < maxTimeDiff {
			continue
		}

		members = append(members, member)
	}

	return
}

func getUsernameOrID(c bot.Client, guildID, userID snowflake.ID) string {
	member, ok := c.Caches().Member(guildID, userID)
	if ok {
		return member.User.Username
	}
	user, err := c.Rest().GetUser(userID)
	if err != nil {
		return "ID:" + userID.String()
	}

	return user.Username
}
func renderLeaveMessage(client bot.Client, guildID, userID snowflake.ID) (string, error) {
	guild, err := client.Rest().GetGuild(guildID, false)
	if err != nil || guild == nil {
		return "", fmt.Errorf("failed to get guild: %w", err)
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return "", fmt.Errorf("failed to get guild settings: %w", err)
	}
	if !settings.LeaveMessageEnabled {
		return "", nil
	}

	user, err := client.Rest().GetUser(userID)
	if err != nil || user == nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	member, err := client.Rest().GetMember(guildID, userID)
	if err != nil || member == nil {
		member = new(discord.Member)
	}
	member.User = *user

	joinleaveInfo := utils.NewMessageTemplateData(*member, guild.Guild)
	contents, err := mustache.RenderRaw(settings.LeaveMessage, true, joinleaveInfo)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return contents, nil
}

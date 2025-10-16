package prune

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
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

	DefaultMemberPermissions: omit.NewPtr(discord.PermissionManageGuild),

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

	_ = e.UpdateMessage(discord.NewMessageUpdateBuilder().SetComponents().Build())

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

	if len(members) == 0 {
		_, err = e.CreateFollowupMessage(ix.EphemeralMessageContent("No members to prune. Original message is likely outdated.").Build())
		return err
	}

	settings, err := model.GetGuildSettings(guildID)
	if err != nil {
		err = fmt.Errorf("failed to retrieve guild settings: %w", err)
		return
	}

	// Prepare the messages that will be shown to moderators and in the join/leave
	// channel if it is enabled.

	modChannelText := ""
	joinleaveText := ""

	for _, member := range members {

		modChannelText += fmt.Sprintf("-# %s\n", getUsernameOrID(e.Client(), guildID, member.UserID))
		text, err := renderLeaveMessage(e.Client(), guildID, member.UserID)
		if err == nil {
			joinleaveText += text + "\n"
		} else {
			joinleaveText += fmt.Sprintf(
				"-# `%s` (ID: `%s`) left the server.\n",
				getUsernameOrID(e.Client(), guildID, member.UserID),
				member.UserID,
			)
		}
	}

	// Prepend a title and append any info messages if they exist.
	modChannelText = fmt.Sprintf(
		"## The following users have been pruned:\n%s\n\n%s",
		modChannelText,
		utils.Iif(messages == "", "", "### Messages:\n"+messages),
	)

	// Split messages up into parts, in case there is a long list of pruned members.
	modChannelTextSplit := utils.SplitStringToLengthByLine(modChannelText, 2000)
	joinleaveTextSplit := utils.SplitStringToLengthByLine(joinleaveText, 2000)

	if settings.ModeratorChannel != 0 {
		// Handle moderator notification of pruned members if a moderator channel is defined.
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Users have been pruned").Build(),
		)
		if err != nil {
			slog.Warn(
				"failed to create prune confirmation message",
				"guild_id", guildID,
				"err", err,
			)
		}

		for _, text := range modChannelTextSplit {
			_, err = e.Client().Rest.CreateMessage(
				settings.ModeratorChannel,
				discord.NewMessageCreateBuilder().
					SetContent(text).Build(),
			)
			if err != nil {
				slog.Warn("failed to create mod prune message", "guild_id", guildID, "err", err)
			}
		}
	} else {
		// if no moderator channel is defined, create an ephemeral message with the
		// information instead
		for _, text := range modChannelTextSplit {
			_, err = e.CreateFollowupMessage(
				ix.EphemeralMessageContent(text).Build(),
			)
			if err != nil {
				slog.Warn("failed to create mod prune message", "guild_id", guildID, "err", err)
			}
		}
	}

	// Post leave messages if they are enabled.
	if settings.JoinLeaveChannel != 0 && settings.LeaveMessageEnabled {
		for _, text := range joinleaveTextSplit {
			_, err := e.Client().Rest.CreateMessage(
				settings.JoinLeaveChannel,
				discord.NewMessageCreateBuilder().
					SetContent(text).Build(),
			)
			if err != nil {
				slog.Warn("failed to create prune join/leave message", "guild_id", guildID, "err", err)
			}
		}
	}

	// Cleanup

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

func kickMembers(client *bot.Client, guildID snowflake.ID, pruneID uuid.UUID) (messages string) {
	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		slog.Error("failed to get guild settings")
		return "failed to kick members"
	}
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

		discordMember, err := client.Rest.GetMember(guildID, member.UserID)
		if err == nil && !slices.Contains(discordMember.RoleIDs, guildSettings.GatekeepPendingRole) {
			_ = model.SetMemberPruned(guildID, member.UserID, false)
			continue // member no longer has the pending role, skip to next.
		}

		err = client.Rest.RemoveMember(guildID, member.UserID)
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
			SetContent(e.Message.Content + "\n\n**Cancelled!**").SetComponents().Build(),
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

	for member := range utils.GetMembersIter(e.Client().Rest, *e.GuildID()) {
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

		if time.Since(utils.RefDefault(member.JoinedAt, time.Now())) < maxTimeDiff {
			continue
		}

		members = append(members, member)
	}

	return
}

func getUsernameOrID(c *bot.Client, guildID, userID snowflake.ID) string {
	member, ok := c.Caches.Member(guildID, userID)
	if ok {
		return member.User.Username
	}
	user, err := c.Rest.GetUser(userID)
	if err != nil {
		return "ID:" + userID.String()
	}

	return user.Username
}
func renderLeaveMessage(client *bot.Client, guildID, userID snowflake.ID) (string, error) {
	guild, err := client.Rest.GetGuild(guildID, false)
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

	user, err := client.Rest.GetUser(userID)
	if err != nil || user == nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	member, err := client.Rest.GetMember(guildID, userID)
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

package prune

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"

	ix "github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rave"
	"github.com/NLLCommunity/heimdallr/utils"
)

var Interactions = rave.Bundle(
	Prune,
	pruneCancelRoute,
	pruneConfirmRoute,
)

type pruneRouteVars struct {
	PruneID uuid.UUID `rave:"pruneID"`
}

var pruneConfirmRoute = rave.ComponentOf[pruneRouteVars](
	"/button/prune-members/confirm/{pruneID}",
).Handle(PruneConfirmHandler)

var pruneCancelRoute = rave.ComponentOf[pruneRouteVars](
	"/button/prune-members/cancel/{pruneID}",
).Handle(PruneCancelHandler)

var Prune = rave.Slash("prune-pending-members", "Prune members.").
	AddNameLocalization(discord.LocaleNorwegian, "fjern-ventende-medlemmer").
	AddDescriptionLocalization(discord.LocaleNorwegian, "Fjern medlemmer.").
	AddContexts(discord.InteractionContextTypeGuild).
	AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
	WithDefaultMemberPermissions(discord.PermissionManageGuild).
	AddOptions(
		rave.OptionInt("days", "The number of days a member has been pending.").
			AddNameLocalization(discord.LocaleNorwegian, "dager").
			AddDescriptionLocalization(discord.LocaleNorwegian, "Antall dager en bruker har vært i serveren og ikke blitt godkjent.").
			WithRequired(true).
			WithMinValue(0).
			WithMaxValue(90),
	).
	Handle(PruneHandler)

func PruneConfirmHandler(e *handler.ComponentEvent) error {
	if e.GuildID() == nil {
		return ix.ErrEventNoGuildID
	}
	guildID := *e.GuildID()

	pruneIDString, ok := e.Vars["pruneID"]
	if !ok {
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred."))
	}

	pruneID, err := uuid.Parse(pruneIDString)
	if err != nil {
		slog.Warn(
			"failed to parse prune ID",
			"guild_id", guildID,
			"prune_id", pruneIDString,
		)
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred."))
	}

	_ = e.UpdateMessage(discord.NewMessageUpdate().WithComponents())

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
		_, err = e.CreateFollowupMessage(ix.EphemeralMessageContent("No members to prune. Original message is likely outdated."))
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
	var joinleaveText strings.Builder

	for _, member := range members {

		modChannelText += fmt.Sprintf("-# %s\n", getUsernameOrID(e.Client(), guildID, member.UserID))
		text, err := renderLeaveMessage(e.Client(), guildID, member.UserID)
		if err == nil {
			joinleaveText.WriteString(text + "\n")
		} else {
			fmt.Fprintf(&joinleaveText, "-# `%s` (ID: `%s`) left the server.\n",
				getUsernameOrID(e.Client(), guildID, member.UserID),
				member.UserID)
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
	joinleaveTextSplit := utils.SplitStringToLengthByLine(joinleaveText.String(), 2000)

	if settings.ModeratorChannel != 0 {
		// Handle moderator notification of pruned members if a moderator channel is defined.
		_, err := e.CreateFollowupMessage(
			ix.EphemeralMessageContent("Users have been pruned"),
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
				discord.NewMessageCreate().
					WithContent(text),
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
				ix.EphemeralMessageContent(text),
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
				discord.NewMessageCreate().
					WithContent(text),
			)
			if err != nil {
				slog.Warn("failed to create prune join/leave message", "guild_id", guildID, "err", err)
			}
		}
	}

	// Cleanup

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
		err := model.SetMemberPruned(guildID, pruneID, member.UserID, true)
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
			_ = model.SetMemberPruned(guildID, pruneID, member.UserID, false)
			continue // member no longer has the pending role, skip to next.
		}

		err = client.Rest.RemoveMember(guildID, member.UserID)
		if err != nil {
			_ = model.SetMemberPruned(guildID, pruneID, member.UserID, false)
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
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred."))
	}

	pruneID, err := uuid.Parse(pruneIDString)
	if err != nil {
		slog.Warn(
			"failed to parse prune ID",
			"guild_id", *e.GuildID(),
			"prune_id", pruneIDString,
		)
		return e.CreateMessage(ix.EphemeralMessageContent("An error occurred."))
	}

	err = model.RemoveMembersByPruneID(pruneID, *e.GuildID())
	if err != nil {
		slog.Warn(
			"failed to discard members pending prune",
			"guild_id", *e.GuildID(),
			"pruneID", pruneID,
		)
		return e.CreateMessage(ix.EphemeralMessageContent("Failed to discard prune list"))
	}

	return e.UpdateMessage(
		discord.NewMessageUpdate().
			WithContent(e.Message.Content + "\n\n**Cancelled!**").WithComponents(),
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
			),
		)
		return err
	}

	if guildSettings.GatekeepPendingRole == 0 {
		return e.CreateMessage(
			ix.EphemeralMessageContent(
				"Failed to prune members: no pending role set. This command will only prune pending members.",
			),
		)
	}

	_ = e.DeferCreateMessage(true)
	prunableMembers, err := getPrunableMembers(e, days, guildSettings)
	if err != nil {
		_, err = e.CreateFollowupMessage(
			ix.EphemeralMessageContent(
				"Failed to prune members: could not get member list.",
			),
		)
		return err
	}

	pruneID := uuid.New()
	messages, err := preparePruneMembers(pruneID, prunableMembers)
	if err != nil {
		slog.Error("Failed to prune members.", "err", err)
		_, err = e.CreateFollowupMessage(ix.EphemeralMessageContent("Failed to prune members: could not process list."))
		return err
	}

	for _, message := range messages {
		if _, err = e.CreateFollowupMessage(message); err != nil {
			return err
		}
	}

	return nil
}

func preparePruneMembers(pruneID uuid.UUID, members []discord.Member) (
	[]discord.MessageCreate, error,
) {
	err := model.AddMembersToBePruned(pruneID, members)
	if err != nil {
		return nil, err
	}

	return buildPruneConfirmMessages(pruneID, members)
}

// buildPruneConfirmMessages builds the confirmation messages for a prune. The
// member list can exceed Discord's 2000 character message limit, so it is
// split across as many messages as needed. The confirm/cancel buttons go on a
// separate short final message, which also leaves room for PruneCancelHandler
// to append to it on cancellation.
func buildPruneConfirmMessages(pruneID uuid.UUID, members []discord.Member) ([]discord.MessageCreate, error) {
	confirmID, err := pruneConfirmRoute.CustomID(pruneRouteVars{PruneID: pruneID})
	if err != nil {
		return nil, err
	}
	cancelID, err := pruneCancelRoute.CustomID(pruneRouteVars{PruneID: pruneID})
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	fmt.Fprintf(&content, "## The following %d members will be pruned and kicked from the server\n", len(members))
	for _, member := range members {
		fmt.Fprintf(&content, "- `%s` (`%s`)\n", member.User.Username, member.User.ID)
	}

	var messages []discord.MessageCreate
	for _, part := range utils.SplitStringToLengthByLine(content.String(), 2000) {
		messages = append(messages, ix.EphemeralMessageContent(part))
	}

	prompt := ix.EphemeralMessageContentf("Prune the %d members listed above?", len(members)).
		AddActionRow(
			discord.NewDangerButton("Prune members", confirmID),
			discord.NewSecondaryButton("Cancel", cancelID),
		)

	return append(messages, prompt), nil
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

package prune

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"

	"github.com/NLLCommunity/heimdallr/globals"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Command("/prune-pending-members", PruneHandler)

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

			MinValue: utils.Ref(3),
			MaxValue: utils.Ref(90),
		},
		discord.ApplicationCommandOptionBool{
			Name:        "dry-run",
			Description: "If true, will show members that would be pruned without actually pruning them. (Default: true)",
		},
	},
}

func PruneHandler(e *handler.CommandEvent) error {
	if e.GuildID() == nil {
		return interactions.ErrEventNoGuildID
	}
	days := e.SlashCommandInteractionData().Int("days")
	dryRun, dryRunOk := e.SlashCommandInteractionData().OptBool("dry-run")
	if !dryRunOk {
		dryRun = true
	}

	guildSettings, err := model.GetGuildSettings(*e.GuildID())
	if err != nil {
		_ = e.CreateMessage(interactions.EphemeralMessageContent(
			"Failed to prune members: could not get guild settings.").Build())
		return err
	}

	if guildSettings.GatekeepPendingRole == 0 {
		return e.CreateMessage(interactions.EphemeralMessageContent(
			"Failed to prune members: no pending role set. This command will only prune pending members.",
		).Build())
	}

	_ = e.DeferCreateMessage(true)
	prunableMembers, err := getPrunableMembers(e, days, guildSettings)
	if err != nil {
		_, err = e.CreateFollowupMessage(interactions.EphemeralMessageContent(
			"Failed to prune members: could not get member list.",
		).Build())
		return err
	}

	if dryRun {
		err = dryRunPruneMembers(e, prunableMembers)
	} else {
		err = pruneMembers(e, *guildSettings, prunableMembers)
	}

	if err != nil {
		slog.Error("Failed to prune members.", "dry_run", dryRun, "err", err)
	}
	return err
}

func pruneMembers(e *handler.CommandEvent, guildSettings model.GuildSettings, members []*discord.Member) error {
	kickedMembers, err := kickMembers(e, members)
	if err != nil {
		return err
	}

	numKicked := len(kickedMembers)
	adminMessage := fmt.Sprintf("Pruned %d members.\n\nMembers: \n", numKicked)
	for _, member := range kickedMembers {
		if member == nil {
			continue
		}
		adminMessage += fmt.Sprintf("-# %s (%s)\n", member.User.Username, member.User.ID)
	}

	if numKicked > 0 && guildSettings.ModeratorChannel != 0 {
		_, err = e.Client().Rest().CreateMessage(
			guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
				SetContent(adminMessage).
				Build(),
		)
		if err != nil {
			slog.Error(
				"Failed to send prune message to moderator channel.",
				"err", err,
				"guild_id", *e.GuildID(),
				"channel_id", guildSettings.ModeratorChannel,
				"user_id", e.User().ID,
			)
		}
	}

	_, err = e.CreateFollowupMessage(interactions.EphemeralMessageContentf(
		"Pruned %d users.", numKicked).Build())
	return err
}

func dryRunPruneMembers(e *handler.CommandEvent, members []*discord.Member) error {
	numKicked := len(members)
	adminMessage := fmt.Sprintf("Dry run: Pruned %d members.\n\nMembers:\n", numKicked)
	for _, member := range members {
		if member == nil {
			continue
		}
		adminMessage += fmt.Sprintf("-# %s (%s)\n", member.User.Username, member.User.ID)
	}

	_, err := e.CreateFollowupMessage(interactions.EphemeralMessageContent(adminMessage).
		Build())
	return err
}

func kickMembers(e *handler.CommandEvent, members []*discord.Member) (kickedMembers []*discord.Member, err error) {
	for _, member := range members {
		if member == nil {
			slog.Warn("Member is nil.")
			continue
		}
		globals.ExcludedFromModKickLog[member.User.ID] = struct{}{}

		err = e.Client().Rest().RemoveMember(
			member.GuildID, member.User.ID,
			rest.WithReason(
				fmt.Sprintf(
					"User pruned with command. Pruned by %s (%s)",
					e.User().Username, e.User().ID,
				),
			),
		)
		if err != nil {
			slog.Error("Failed to prune member.", "err", err, "user_id", member.User.ID)
			return
		}
		kickedMembers = append(kickedMembers, member)
	}
	return
}

func getPrunableMembers(
	e *handler.CommandEvent, days int, guildSettings *model.GuildSettings,
) (members []*discord.Member, err error) {
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

		members = append(members, &member)
	}

	return
}

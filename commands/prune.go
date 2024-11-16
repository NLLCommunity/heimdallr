package commands

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"

	"github.com/myrkvi/heimdallr/globals"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
)

var PruneDryRunCommand = discord.SlashCommandCreate{
	Name: "prune-pending-members-dry-run",
	NameLocalizations: map[discord.Locale]string{
		discord.LocaleNorwegian: "fjern-ventende-medlemmer-dry-run",
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

func PruneDryRunHandler(e *handler.CommandEvent) error {
	if e.GuildID() == nil {
		return ErrEventNoGuildID
	}
	days := e.SlashCommandInteractionData().Int("days")

	guildSettings, err := model.GetGuildSettings(*e.GuildID())
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to prune members: could not get guild settings.").
			Build())
		return err
	}

	if guildSettings.GatekeepPendingRole == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to prune members: no pending role set. This command will only prune pending members.").
			Build())
	}

	_ = e.DeferCreateMessage(true)

	prunableMembers, err := getPrunableMembers(e, days, guildSettings)
	if err != nil {
		_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to prune members: could not get member list.").
			Build())
		return err
	}

	numKicked := len(prunableMembers)

	adminMessage := fmt.Sprintf("Dry run: pruned %d members.\n\nMembers:\n", numKicked)

	for _, member := range prunableMembers {
		if member == nil {
			continue
		}

		adminMessage += fmt.Sprintf("-# %s (%s)\n", member.User.Username, member.User.ID)
	}

	_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContent(adminMessage).
		Build())
	return err
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

func PruneHandler(e *handler.CommandEvent) error {
	if e.GuildID() == nil {
		return ErrEventNoGuildID
	}
	days := e.SlashCommandInteractionData().Int("days")

	guildSettings, err := model.GetGuildSettings(*e.GuildID())
	if err != nil {
		_ = e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to prune members: could not get guild settings.").
			Build())
		return err
	}

	if guildSettings.GatekeepPendingRole == 0 {
		return e.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to prune members: no pending role set. This command will only prune pending members.").
			Build())
	}

	_ = e.DeferCreateMessage(true)

	var kickedMembers []*discord.Member

	prunableMembers, err := getPrunableMembers(e, days, guildSettings)
	if err != nil {
		_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Failed to prune members: could not get member list.").
			Build())
		return err
	}

	for _, member := range prunableMembers {

		globals.ExcludedFromModKickLog[member.User.ID] = struct{}{}
		kickedMembers = append(kickedMembers, member)

		err = e.Client().Rest().RemoveMember(*e.GuildID(), member.User.ID,
			rest.WithReason(
				fmt.Sprintf("User pruned with command. Pruned by: %s (%s)",
					e.User().Username, e.User().ID)))
		if err != nil {
			slog.Error("Failed to prune member.", "err", err, "user_id", member.User.ID)
			_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Failed to prune members: failed to remove member.").
				Build())
			return err
		}
	}

	numKicked := len(kickedMembers)

	adminMessage := fmt.Sprintf("Pruned %d members.\n\nMembers:\n", numKicked)

	for _, member := range kickedMembers {
		if member == nil {
			continue
		}

		adminMessage += fmt.Sprintf("-# %s (%s)\n", member.User.Username, member.User.ID)
	}

	if numKicked > 0 && guildSettings.ModeratorChannel != 0 {
		_, err = e.Client().Rest().CreateMessage(guildSettings.ModeratorChannel, discord.NewMessageCreateBuilder().
			SetContent(adminMessage).
			SetEphemeral(true).
			Build())
		if err != nil {
			slog.Error("Failed to send prune message to moderator channel.",
				"err", err,
				"guild_id", *e.GuildID(),
				"channel_id", guildSettings.ModeratorChannel,
				"user_id", e.User().ID)
		}
	}

	_ = days

	_, err = e.CreateFollowupMessage(discord.NewMessageCreateBuilder().
		SetEphemeral(true).
		SetContentf("Pruned %d users.", numKicked).
		Build())
	return err
}

func getPrunableMembers(e *handler.CommandEvent, days int, guildSettings *model.GuildSettings) (members []*discord.Member, err error) {
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

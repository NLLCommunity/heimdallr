package ban

import (
	"fmt"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/json"

	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/utils"
)

func Register(r *handler.Mux) []discord.ApplicationCommandCreate {
	r.Route(
		"/ban", func(r handler.Router) {
			r.Command("/with-message", BanWithMessageHandler)
			r.Command("/until", BanUntilHandler)
		},
	)

	return []discord.ApplicationCommandCreate{BanCommand}
}

var BanCommand = discord.SlashCommandCreate{
	Name:                     "ban",
	Description:              "Ban a user from the server",
	DMPermission:             utils.Ref(false),
	DefaultMemberPermissions: json.NewNullablePtr(discord.PermissionBanMembers),
	Options: []discord.ApplicationCommandOption{
		banWithMessageSubCommand,
		banUntilSubcommand,
	},
}

func banHandlerInner(e *handler.CommandEvent, user discord.User, sendReason bool, reason, duration string) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}
	dur, err := utils.ParseLongDuration(duration)
	var banExp string
	if err == nil {
		banExp = fmt.Sprintf("<t:%d:R>", time.Now().Add(dur).Unix())
	} else {
		banExp = duration
	}

	failedToMessage := false
	if sendReason || duration != "" {
		mc := discord.NewMessageCreateBuilder().
			SetContentf(
				"You have been banned from %s.\n"+
					utils.Iif(duration != "", fmt.Sprintf("This ban will expire %s.\n", banExp), "")+
					utils.Iif(
						sendReason,
						fmt.Sprintf("Along with the ban, this message was added:\n\n %s\n\n", reason), "",
					)+
					"(You cannot respond to this message.)",
				guild.Name,
			).Build()

		_, err := interactions.SendDirectMessage(e.Client(), user, mc)
		if err != nil {
			failedToMessage = true
		}
	}

	err = e.Client().Rest().AddBan(
		guild.ID, user.ID, 0,
		rest.WithReason(
			fmt.Sprintf(
				"Banned by: %s (%s) %s, with message: %s",
				e.User().Username, e.User().ID,
				utils.Iif(duration != "", fmt.Sprintf("for %s", duration), ""),
				reason,
			),
		),
	)
	if err != nil {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Failed to ban user").
				Build(),
		)
	}

	if failedToMessage {
		return e.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("User was banned but message failed to send.").
				Build(),
		)
	}

	return e.CreateMessage(
		discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("User was banned.").
			Build(),
	)

}

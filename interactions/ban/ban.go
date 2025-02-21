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

type banHandlerData struct {
	user       *discord.User
	guild      *discord.Guild
	duration   string
	reason     string
	sendReason bool
}

func banHandlerInner(e *handler.CommandEvent, data banHandlerData) error {
	guild, isGuild := e.Guild()
	if !isGuild {
		return interactions.ErrEventNoGuildID
	}

	failedToMessage := false
	if data.sendReason || data.duration != "" {
		mc := createBanDMMessage(data)
		_, err := interactions.SendDirectMessage(e.Client(), *data.user, mc)
		if err != nil {
			failedToMessage = true
		}
	}

	err := e.Client().Rest().AddBan(
		guild.ID, data.user.ID, 0,
		rest.WithReason(
			fmt.Sprintf(
				"Banned by: %s (%s) %s, with message: %s",
				e.User().Username, e.User().ID,
				utils.Iif(data.duration != "", fmt.Sprintf("for %s", data.duration), ""),
				data.reason,
			),
		),
	)
	if err != nil {
		return interactions.MessageEphWithContentf(e, "Failed to ban user")
	}
	if failedToMessage {
		return interactions.MessageEphWithContentf(e, "User was banned but message failed to send.")
	}

	return interactions.MessageEphWithContentf(e, "User was banned.")

}

func createBanDMMessage(data banHandlerData) discord.MessageCreate {
	banExp := durationToRelTimestamp(data.duration)

	expiryText := fmt.Sprintf("This ban will expire %s.", banExp)
	reasonText := fmt.Sprintf(
		"Along with the ban, this message was added:\n\n %s\n\n",
		data.reason,
	)

	return discord.NewMessageCreateBuilder().
		SetContentf(
			"You have been banned from %s.\n%s%s\n\n(You cannot respond to this message)",
			data.guild.Name,
			utils.Iif(data.duration != "", expiryText, ""),
			utils.Iif(data.sendReason, reasonText, ""),
		).Build()
}

func durationToRelTimestamp(duration string) string {
	dur, err := utils.ParseLongDuration(duration)
	if err != nil {
		return duration
	}
	return fmt.Sprintf("<t:%d:R>", time.Now().Add(dur).Unix())
}

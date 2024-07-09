package listeners

import (
	"github.com/cbroglie/mustache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/utils"
	"log/slog"
)

type joinleaveInfo struct {
	User   joinLeaveUserInfo
	Server joinLeaveServerInfo
}

type joinLeaveUserInfo struct {
	Username      string
	GlobalName    string
	ServerName    string
	ResolvedName  string
	Mention       string
	Discriminator uint8
	IsBot         bool
	ID            snowflake.ID
}

type joinLeaveServerInfo struct {
	Name string
	ID   snowflake.ID
}

func OnUserJoin(e *events.GuildMemberJoin) {
	guildID := e.GuildID
	guild, err := e.Client().Rest().GetGuild(guildID, false)
	if err != nil {
		return
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return
	}

	if !guildSettings.JoinMessageEnabled {
		return
	}

	joinLeaveChannel := guildSettings.JoinLeaveChannel
	if joinLeaveChannel == 0 && guild.SystemChannelID != nil {
		joinLeaveChannel = *guild.SystemChannelID
	}
	if joinLeaveChannel == 0 {
		return
	}

	joinleaveInfo := joinleaveInfo{
		User: joinLeaveUserInfo{
			Username: utils.Iif(
				e.Member.User.Discriminator == "0",
				e.Member.User.Username,
				e.Member.User.Username+"#"+e.Member.User.Discriminator,
			),
			GlobalName:   utils.RefDefault(e.Member.User.GlobalName, ""),
			ServerName:   utils.RefDefault(e.Member.Nick, ""),
			ResolvedName: e.Member.EffectiveName(),
			Mention:      e.Member.Mention(),
		},
		Server: joinLeaveServerInfo{
			Name: guild.Name,
			ID:   guildID,
		},
	}

	contents, err := mustache.RenderRaw(guildSettings.JoinMessage, true, joinleaveInfo)
	if err != nil {
		slog.Error("Failed to render join message template.",
			"err", err,
			"guild_id", guildID,
		)
		return
	}

	_, err = e.Client().Rest().CreateMessage(joinLeaveChannel, discord.NewMessageCreateBuilder().SetContent(contents).Build())
}

func OnUserLeave(e *events.GuildMemberLeave) {
	guildID := e.GuildID
	guild, err := e.Client().Rest().GetGuild(guildID, false)
	if err != nil {
		return
	}

	guildSettings, err := model.GetGuildSettings(guildID)
	if err != nil {
		return
	}

	if !guildSettings.LeaveMessageEnabled {
		return
	}

	joinLeaveChannel := guildSettings.JoinLeaveChannel
	if joinLeaveChannel == 0 && guild.SystemChannelID != nil {
		joinLeaveChannel = *guild.SystemChannelID
	}
	if joinLeaveChannel == 0 {
		return
	}

	joinleaveInfo := joinleaveInfo{
		User: joinLeaveUserInfo{
			Username: utils.Iif(
				e.Member.User.Discriminator == "0",
				e.Member.User.Username,
				e.Member.User.Username+"#"+e.Member.User.Discriminator,
			),
			GlobalName:   utils.RefDefault(e.Member.User.GlobalName, ""),
			ServerName:   utils.RefDefault(e.Member.Nick, ""),
			ResolvedName: e.Member.EffectiveName(),
			Mention:      e.Member.Mention(),
		},
		Server: joinLeaveServerInfo{
			Name: guild.Name,
			ID:   guildID,
		},
	}

	contents, err := mustache.RenderRaw(guildSettings.LeaveMessage, true, joinleaveInfo)
	if err != nil {
		slog.Error("Failed to render leave message template.",
			"err", err,
			"guild_id", guildID,
		)
		return
	}

	_, err = e.Client().Rest().CreateMessage(joinLeaveChannel, discord.NewMessageCreateBuilder().SetContent(contents).Build())
}

package utils

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type MessageTemplateData struct {
	User   TemplateUserData
	Server TemplateGuildData
}

type TemplateUserData struct {
	Username      string
	GlobalName    string
	ServerName    string
	ResolvedName  string
	Mention       string
	Discriminator uint8
	IsBot         bool
	ID            snowflake.ID
}

type TemplateGuildData struct {
	Name string
	ID   snowflake.ID
}

func NewMessageTemplateData(user discord.Member, guild discord.Guild) MessageTemplateData {
	return MessageTemplateData{
		User: TemplateUserData{
			Username: Iif(
				user.User.Discriminator == "0",
				user.User.Username,
				user.User.Username+"#"+user.User.Discriminator,
			),
			GlobalName:   RefDefault(user.User.GlobalName, ""),
			ServerName:   RefDefault(user.Nick, ""),
			ResolvedName: user.EffectiveName(),
			Mention:      user.Mention(),
			ID:           user.User.ID,
		},
		Server: TemplateGuildData{
			Name: guild.Name,
			ID:   guild.ID,
		},
	}
}

var MessageTemplateInfo = "The following placeholders can be used in join/leave/approval messages " +
	"and will be replaced with the appropriate values." +
	"\n" +
	"\n" +
	"**Username:** `{{User.Username}}` will show as \"Username#1234\" or  \"username\"\n" +
	"**Global name:** `{{User.GlobalName}}` will show the user's global display name\n" +
	"**Server name:** `{{User.ServerName}}` will show the user's server nickname, if any\n" +
	"**Resolved name:** `{{User.ResolvedName}}` will show the user's resolved name, which is" +
	"the server nickname if set, otherwise the global name, or username if neither is set\n" +
	"**Mention:** `{{User.Mention}}` will mention the user if it is used\n" +
	"**User ID:** `{{User.ID}}` will show the user's ID\n" +
	"**Server name:** `{{Server.Name}}` will show the server name\n" +
	"**Server ID:** `{{Server.ID}}` will show the server ID\n"

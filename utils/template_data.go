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
		},
		Server: TemplateGuildData{
			Name: guild.Name,
			ID:   guild.ID,
		},
	}
}

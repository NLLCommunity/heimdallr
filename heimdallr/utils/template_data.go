package utils

import (
	"fmt"
	"strings"

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

// MessageTemplatePlaceholder describes a single template placeholder.
type MessageTemplatePlaceholder struct {
	Placeholder string
	Description string
}

// MessageTemplatePlaceholders is the structured list of available placeholders.
var MessageTemplatePlaceholders = []MessageTemplatePlaceholder{
	{"{{User.Username}}", `Username (e.g. "Username#1234" or "username")`},
	{"{{User.GlobalName}}", "The user's global display name"},
	{"{{User.ServerName}}", "The user's server nickname, if any"},
	{"{{User.ResolvedName}}", "Server nickname, global name, or username (first available)"},
	{"{{User.Mention}}", "Mentions the user"},
	{"{{User.ID}}", "The user's ID"},
	{"{{Server.Name}}", "The server name"},
	{"{{Server.ID}}", "The server ID"},
}

var MessageTemplateInfo = func() string {
	var b strings.Builder
	b.WriteString("The following placeholders can be used in join/leave/approval messages " +
		"and will be replaced with the appropriate values.\n\n")
	for _, p := range MessageTemplatePlaceholders {
		fmt.Fprintf(&b, "`%s` — %s\n", p.Placeholder, p.Description)
	}
	return b.String()
}()

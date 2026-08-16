package rave_test

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

func muxHasCommandRoute(mux handler.Router, path string) bool {
	return mux.Match(
		path,
		discord.InteractionTypeApplicationCommand,
		int(discord.ApplicationCommandTypeSlash),
	)
}

func TestSlashCommandRegisterRegistersRootHandler(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("note", "Add a note").
		Handle(noopCommandHandler).
		AddOptions(rave.OptionString("text", "Note text"))

	built := command.Register(mux)

	require.Equal(t, "note", built.CommandName())
	require.True(t, muxHasCommandRoute(mux, "/note"))
}

func TestSlashCommandRegisterRegistersDirectSubcommandHandler(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("info", "Show configuration").
				Handle(noopCommandHandler),
		)

	command.Register(mux)

	require.False(t, muxHasCommandRoute(mux, "/admin"))
	require.True(t, muxHasCommandRoute(mux, "/admin/info"))
}

func TestSlashCommandRegisterRegistersGroupedSubcommandHandler(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("role", "Role commands").
		AddOptions(
			rave.SubCommandGroup("member", "Member role commands").
				AddOptions(
					rave.SubCommand("add", "Add a role").
						Handle(noopCommandHandler),
				),
		)

	command.Register(mux)

	require.False(t, muxHasCommandRoute(mux, "/role"))
	require.False(t, muxHasCommandRoute(mux, "/role/member"))
	require.True(t, muxHasCommandRoute(mux, "/role/member/add"))
}

func TestSlashCommandRegisterRejectsLeafWithoutHandler(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("note", "Add a note")

	require.Panics(t, func() {
		command.Register(mux)
	})
}

func TestSlashCommandRegisterRejectsHandlerOnContainer(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("admin", "Administrative commands").
		Handle(noopCommandHandler).
		AddOptions(
			rave.SubCommand("info", "Show configuration").
				Handle(noopCommandHandler),
		)

	require.Panics(t, func() {
		command.Register(mux)
	})
}

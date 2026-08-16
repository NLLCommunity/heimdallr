package rave_test

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

func muxHasCommandRoute(mux handler.Router, path string) bool {
	return muxHasCommandRouteForType(mux, path, discord.ApplicationCommandTypeSlash)
}

func muxHasCommandRouteForType(
	mux handler.Router,
	path string,
	commandType discord.ApplicationCommandType,
) bool {
	return mux.Match(
		path,
		discord.InteractionTypeApplicationCommand,
		int(commandType),
	)
}

func noopSlashCommandHandler(discord.SlashCommandInteractionData, *handler.CommandEvent) error {
	return nil
}

func noopUserCommandHandler(discord.UserCommandInteractionData, *handler.CommandEvent) error {
	return nil
}

func noopMessageCommandHandler(discord.MessageCommandInteractionData, *handler.CommandEvent) error {
	return nil
}

func TestSlashCommandRegisterRegistersTypedHandlerOnlyForSlashCommands(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("note", "Add a note").
		HandleSlash(noopSlashCommandHandler)

	command.Register(mux)

	require.True(t, muxHasCommandRouteForType(mux, "/note", discord.ApplicationCommandTypeSlash))
	require.False(t, muxHasCommandRouteForType(mux, "/note", discord.ApplicationCommandTypeUser))
}

func TestSlashCommandRegisterRegistersTypedSubcommandHandler(t *testing.T) {
	mux := handler.New()
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("info", "Show configuration").
				HandleSlash(noopSlashCommandHandler),
		)

	command.Register(mux)

	require.True(t, muxHasCommandRouteForType(mux, "/admin/info", discord.ApplicationCommandTypeSlash))
	require.False(t, muxHasCommandRouteForType(mux, "/admin/info", discord.ApplicationCommandTypeUser))
}

func TestSlashCommandHandlerConfigurationRejectsMixedStyles(t *testing.T) {
	tests := []struct {
		name      string
		configure func()
	}{
		{
			name: "generic then typed",
			configure: func() {
				rave.Slash("note", "Add a note").
					Handle(noopCommandHandler).
					HandleSlash(noopSlashCommandHandler)
			},
		},
		{
			name: "typed then generic",
			configure: func() {
				rave.Slash("note", "Add a note").
					HandleSlash(noopSlashCommandHandler).
					Handle(noopCommandHandler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Panics(t, tt.configure)
		})
	}
}

func TestSubcommandHandlerConfigurationRejectsMixedStyles(t *testing.T) {
	require.Panics(t, func() {
		rave.SubCommand("info", "Show configuration").
			Handle(noopCommandHandler).
			HandleSlash(noopSlashCommandHandler)
	})
	require.Panics(t, func() {
		rave.SubCommand("info", "Show configuration").
			HandleSlash(noopSlashCommandHandler).
			Handle(noopCommandHandler)
	})
}

func TestUserCommandRegisterRegistersGenericHandler(t *testing.T) {
	mux := handler.New()
	command := rave.UserCommand("Approve").Handle(noopCommandHandler)

	built := command.Register(mux)

	require.IsType(t, discord.UserCommandCreate{}, built)
	require.True(t, muxHasCommandRouteForType(mux, "/Approve", discord.ApplicationCommandTypeUser))
	require.True(t, muxHasCommandRouteForType(mux, "/Approve", discord.ApplicationCommandTypeSlash))
}

func TestUserCommandRegisterRegistersTypedHandlerOnlyForUserCommands(t *testing.T) {
	mux := handler.New()
	command := rave.UserCommand("Approve").HandleUser(noopUserCommandHandler)

	command.Register(mux)

	require.True(t, muxHasCommandRouteForType(mux, "/Approve", discord.ApplicationCommandTypeUser))
	require.False(t, muxHasCommandRouteForType(mux, "/Approve", discord.ApplicationCommandTypeSlash))
}

func TestUserCommandHandlerConfigurationRejectsMixedStyles(t *testing.T) {
	tests := []struct {
		name      string
		configure func()
	}{
		{
			name: "generic then typed",
			configure: func() {
				rave.UserCommand("Approve").
					Handle(noopCommandHandler).
					HandleUser(noopUserCommandHandler)
			},
		},
		{
			name: "typed then generic",
			configure: func() {
				rave.UserCommand("Approve").
					HandleUser(noopUserCommandHandler).
					Handle(noopCommandHandler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Panics(t, tt.configure)
		})
	}
}

func TestMessageCommandRegisterRegistersGenericHandler(t *testing.T) {
	mux := handler.New()
	command := rave.MessageCommand("Report Message").Handle(noopCommandHandler)

	built := command.Register(mux)

	require.IsType(t, discord.MessageCommandCreate{}, built)
	require.True(t, muxHasCommandRouteForType(mux, "/Report Message", discord.ApplicationCommandTypeMessage))
	require.True(t, muxHasCommandRouteForType(mux, "/Report Message", discord.ApplicationCommandTypeSlash))
}

func TestMessageCommandRegisterRegistersTypedHandlerOnlyForMessageCommands(t *testing.T) {
	mux := handler.New()
	command := rave.MessageCommand("Report Message").HandleMessage(noopMessageCommandHandler)

	command.Register(mux)

	require.True(t, muxHasCommandRouteForType(mux, "/Report Message", discord.ApplicationCommandTypeMessage))
	require.False(t, muxHasCommandRouteForType(mux, "/Report Message", discord.ApplicationCommandTypeSlash))
}

func TestMessageCommandHandlerConfigurationRejectsMixedStyles(t *testing.T) {
	tests := []struct {
		name      string
		configure func()
	}{
		{
			name: "generic then typed",
			configure: func() {
				rave.MessageCommand("Report Message").
					Handle(noopCommandHandler).
					HandleMessage(noopMessageCommandHandler)
			},
		},
		{
			name: "typed then generic",
			configure: func() {
				rave.MessageCommand("Report Message").
					HandleMessage(noopMessageCommandHandler).
					Handle(noopCommandHandler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Panics(t, tt.configure)
		})
	}
}

func TestHandlerConfigurationAllowsRepeatedSameStyleSetters(t *testing.T) {
	tests := []struct {
		name      string
		configure func()
	}{
		{
			name: "slash generic",
			configure: func() {
				rave.Slash("note", "Add a note").
					Handle(noopCommandHandler).
					Handle(noopCommandHandler)
			},
		},
		{
			name: "slash typed",
			configure: func() {
				rave.Slash("note", "Add a note").
					HandleSlash(noopSlashCommandHandler).
					HandleSlash(noopSlashCommandHandler)
			},
		},
		{
			name: "subcommand generic",
			configure: func() {
				rave.SubCommand("info", "Show configuration").
					Handle(noopCommandHandler).
					Handle(noopCommandHandler)
			},
		},
		{
			name: "subcommand typed",
			configure: func() {
				rave.SubCommand("info", "Show configuration").
					HandleSlash(noopSlashCommandHandler).
					HandleSlash(noopSlashCommandHandler)
			},
		},
		{
			name: "user generic",
			configure: func() {
				rave.UserCommand("Approve").
					Handle(noopCommandHandler).
					Handle(noopCommandHandler)
			},
		},
		{
			name: "user typed",
			configure: func() {
				rave.UserCommand("Approve").
					HandleUser(noopUserCommandHandler).
					HandleUser(noopUserCommandHandler)
			},
		},
		{
			name: "message generic",
			configure: func() {
				rave.MessageCommand("Report Message").
					Handle(noopCommandHandler).
					Handle(noopCommandHandler)
			},
		},
		{
			name: "message typed",
			configure: func() {
				rave.MessageCommand("Report Message").
					HandleMessage(noopMessageCommandHandler).
					HandleMessage(noopMessageCommandHandler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, tt.configure)
		})
	}
}

func TestHandlerConfigurationIgnoresNilAlternativeStyle(t *testing.T) {
	tests := []struct {
		name      string
		configure func()
	}{
		{
			name: "slash typed nil",
			configure: func() {
				rave.Slash("note", "Add a note").
					Handle(noopCommandHandler).
					HandleSlash(nil)
			},
		},
		{
			name: "slash generic nil",
			configure: func() {
				rave.Slash("note", "Add a note").
					HandleSlash(noopSlashCommandHandler).
					Handle(nil)
			},
		},
		{
			name: "subcommand typed nil",
			configure: func() {
				rave.SubCommand("info", "Show configuration").
					Handle(noopCommandHandler).
					HandleSlash(nil)
			},
		},
		{
			name: "subcommand generic nil",
			configure: func() {
				rave.SubCommand("info", "Show configuration").
					HandleSlash(noopSlashCommandHandler).
					Handle(nil)
			},
		},
		{
			name: "user typed nil",
			configure: func() {
				rave.UserCommand("Approve").
					Handle(noopCommandHandler).
					HandleUser(nil)
			},
		},
		{
			name: "user generic nil",
			configure: func() {
				rave.UserCommand("Approve").
					HandleUser(noopUserCommandHandler).
					Handle(nil)
			},
		},
		{
			name: "message typed nil",
			configure: func() {
				rave.MessageCommand("Report Message").
					Handle(noopCommandHandler).
					HandleMessage(nil)
			},
		},
		{
			name: "message generic nil",
			configure: func() {
				rave.MessageCommand("Report Message").
					HandleMessage(noopMessageCommandHandler).
					Handle(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, tt.configure)
		})
	}
}

func TestContextCommandRegisterRejectsMissingHandler(t *testing.T) {
	tests := []struct {
		name     string
		register func(handler.Router)
	}{
		{
			name: "user command",
			register: func(router handler.Router) {
				rave.UserCommand("Approve").Register(router)
			},
		},
		{
			name: "message command",
			register: func(router handler.Router) {
				rave.MessageCommand("Report Message").Register(router)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Panics(t, func() {
				tt.register(handler.New())
			})
		})
	}
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

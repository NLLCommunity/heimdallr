package rave_test

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

func noopCommandHandler(*handler.CommandEvent) error {
	return nil
}

func TestUserCommandBuildsContextCommandMetadata(t *testing.T) {
	command := rave.UserCommand("Approve").
		AddNameLocalization(discord.LocaleGerman, "Genehmigen").
		WithDefaultMemberPermissions(discord.PermissionKickMembers).
		AddIntegrationTypes(discord.ApplicationIntegrationTypeGuildInstall).
		AddContexts(discord.InteractionContextTypeGuild).
		WithNSFW(true).
		Handle(noopCommandHandler)

	built := command.Build()

	nsfw := true
	require.Equal(t, discord.UserCommandCreate{
		Name: "Approve",
		NameLocalizations: map[discord.Locale]string{
			discord.LocaleGerman: "Genehmigen",
		},
		DefaultMemberPermissions: omit.NewPtr(discord.PermissionKickMembers),
		IntegrationTypes: []discord.ApplicationIntegrationType{
			discord.ApplicationIntegrationTypeGuildInstall,
		},
		Contexts: []discord.InteractionContextType{
			discord.InteractionContextTypeGuild,
		},
		NSFW: &nsfw,
	}, built)
}

func TestMessageCommandBuildsContextCommandMetadata(t *testing.T) {
	command := rave.MessageCommand("Report Message").
		WithNameLocalizations(map[discord.Locale]string{
			discord.LocaleGerman: "Nachricht melden",
		}).
		WithIntegrationTypes([]discord.ApplicationIntegrationType{
			discord.ApplicationIntegrationTypeGuildInstall,
		}).
		WithContexts([]discord.InteractionContextType{
			discord.InteractionContextTypeGuild,
		}).
		Handle(noopCommandHandler)

	built := command.Build()

	require.Equal(t, discord.MessageCommandCreate{
		Name:                     "Report Message",
		DefaultMemberPermissions: omit.New[*discord.Permissions](nil),
		NameLocalizations: map[discord.Locale]string{
			discord.LocaleGerman: "Nachricht melden",
		},
		IntegrationTypes: []discord.ApplicationIntegrationType{
			discord.ApplicationIntegrationTypeGuildInstall,
		},
		Contexts: []discord.InteractionContextType{
			discord.InteractionContextTypeGuild,
		},
	}, built)
}

func TestContextCommandBuildRejectsInvalidNames(t *testing.T) {
	tests := []struct {
		name  string
		build func() discord.ApplicationCommandCreate
	}{
		{
			name: "empty user command name",
			build: func() discord.ApplicationCommandCreate {
				return rave.UserCommand("").Build()
			},
		},
		{
			name: "long message command name",
			build: func() discord.ApplicationCommandCreate {
				return rave.MessageCommand(strings.Repeat("a", 33)).Build()
			},
		},
		{
			name: "empty localized name",
			build: func() discord.ApplicationCommandCreate {
				return rave.UserCommand("Approve").
					AddNameLocalization(discord.LocaleGerman, "").
					Build()
			},
		},
		{
			name: "invalid UTF-8",
			build: func() discord.ApplicationCommandCreate {
				return rave.MessageCommand(string([]byte{0xff})).Build()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Panics(t, func() {
				tt.build()
			})
		})
	}
}

func TestContextCommandNameLengthCountsUnicodeCharacters(t *testing.T) {
	require.NotPanics(t, func() {
		rave.UserCommand(strings.Repeat("å", 32)).Build()
	})
	require.Panics(t, func() {
		rave.MessageCommand(strings.Repeat("å", 33)).Build()
	})
}

func TestSlashCommandBuildsPrimitiveOptions(t *testing.T) {
	command := rave.Slash("note", "Add a note").
		Handle(noopCommandHandler).
		AddOptions(
			rave.OptionString("text", "The note text").
				WithRequired(true),
		)

	built := command.Build()

	require.Equal(t, "note", built.CommandName())
	slash, ok := built.(discord.SlashCommandCreate)
	require.True(t, ok)
	require.Equal(t, "Add a note", slash.Description)
	require.Equal(t, []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionString{
			Name:        "text",
			Description: "The note text",
			Required:    true,
		},
	}, slash.Options)
}

func TestSlashCommandBuildsDirectSubcommand(t *testing.T) {
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("info", "Show configuration").
				Handle(noopCommandHandler).
				AddOptions(rave.OptionBool("verbose", "Show extra details")),
		)

	slash := command.Build().(discord.SlashCommandCreate)

	require.Equal(t, []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommand{
			Name:        "info",
			Description: "Show configuration",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "verbose",
					Description: "Show extra details",
				},
			},
		},
	}, slash.Options)
}

func TestSlashCommandBuildsGroupedSubcommand(t *testing.T) {
	command := rave.Slash("role", "Role commands").
		AddOptions(
			rave.SubCommandGroup("member", "Member role commands").
				AddOptions(
					rave.SubCommand("add", "Add a role").
						Handle(noopCommandHandler).
						AddOptions(rave.OptionRole("role", "Role to add")),
				),
		)

	slash := command.Build().(discord.SlashCommandCreate)

	require.Equal(t, []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionSubCommandGroup{
			Name:        "member",
			Description: "Member role commands",
			Options: []discord.ApplicationCommandOptionSubCommand{
				{
					Name:        "add",
					Description: "Add a role",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionRole{
							Name:        "role",
							Description: "Role to add",
						},
					},
				},
			},
		},
	}, slash.Options)
}

func TestSlashCommandBuildRejectsDuplicateOptionNames(t *testing.T) {
	command := rave.Slash("search", "Search messages").
		AddOptions(rave.OptionString("query", "First query")).
		AddOptions(rave.OptionString("query", "Second query"))

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsMixedPrimitiveAndSubcommandOptions(t *testing.T) {
	command := rave.Slash("search", "Search messages").
		AddOptions(rave.OptionString("query", "Search query")).
		AddOptions(
			rave.SubCommand("recent", "Search recent messages").
				Handle(noopCommandHandler),
		)

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsRequiredOptionAfterOptionalOption(t *testing.T) {
	command := rave.Slash("search", "Search messages").
		AddOptions(rave.OptionString("channel", "Optional channel")).
		AddOptions(
			rave.OptionString("query", "Required query").
				WithRequired(true),
		)

	require.Panics(t, func() {
		command.Build()
	})
}

func TestSlashCommandBuildRejectsNestedSubcommand(t *testing.T) {
	command := rave.Slash("admin", "Administrative commands").
		AddOptions(
			rave.SubCommand("parent", "Invalid parent").
				AddOptions(
					rave.SubCommand("child", "Invalid child").
						Handle(noopCommandHandler),
				),
		)

	require.Panics(t, func() {
		command.Build()
	})
}

func commandStringOptions(count int) []rave.CommandOption {
	options := make([]rave.CommandOption, count)
	for i := range options {
		options[i] = rave.OptionString(fmt.Sprintf("option%d", i), "Description")
	}
	return options
}

func commandSubcommandGroup(count int) *rave.SlashCommandBuilder {
	group := rave.SubCommandGroup("group", "Description")
	for i := range count {
		group.AddOptions(rave.SubCommand(fmt.Sprintf("subcommand%d", i), "Description"))
	}
	return rave.Slash("root", "Description").AddOptions(group)
}

func TestSlashCommandOptionCountBounds(t *testing.T) {
	tests := []struct {
		name      string
		optionCnt int
		build     func(int)
		panics    bool
	}{
		{
			name:      "root accepts 25 options",
			optionCnt: 25,
			build: func(count int) {
				rave.Slash("root", "Description").AddOptions(commandStringOptions(count)...).Build()
			},
		},
		{
			name:      "root rejects 26 options",
			optionCnt: 26,
			build: func(count int) {
				rave.Slash("root", "Description").AddOptions(commandStringOptions(count)...).Build()
			},
			panics: true,
		},
		{
			name:      "subcommand accepts 25 options",
			optionCnt: 25,
			build: func(count int) {
				rave.Slash("root", "Description").AddOptions(
					rave.SubCommand("subcommand", "Description").AddOptions(commandStringOptions(count)...),
				).Build()
			},
		},
		{
			name:      "subcommand rejects 26 options",
			optionCnt: 26,
			build: func(count int) {
				rave.Slash("root", "Description").AddOptions(
					rave.SubCommand("subcommand", "Description").AddOptions(commandStringOptions(count)...),
				).Build()
			},
			panics: true,
		},
		{
			name:      "group accepts 25 subcommands",
			optionCnt: 25,
			build: func(count int) {
				commandSubcommandGroup(count).Build()
			},
		},
		{
			name:      "group rejects 26 subcommands",
			optionCnt: 26,
			build: func(count int) {
				commandSubcommandGroup(count).Build()
			},
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				require.Panics(t, func() { tt.build(tt.optionCnt) })
				return
			}
			require.NotPanics(t, func() { tt.build(tt.optionCnt) })
		})
	}
}

func TestStringOptionLengthBounds(t *testing.T) {
	tests := []struct {
		name   string
		option rave.CommandOption
		panics bool
	}{
		{name: "min length 0", option: rave.OptionString("value", "Description").WithMinLength(0)},
		{name: "min length 6000", option: rave.OptionString("value", "Description").WithMinLength(6000)},
		{name: "min length 6001", option: rave.OptionString("value", "Description").WithMinLength(6001), panics: true},
		{name: "max length 1", option: rave.OptionString("value", "Description").WithMaxLength(1)},
		{name: "max length 6000", option: rave.OptionString("value", "Description").WithMaxLength(6000)},
		{name: "max length 6001", option: rave.OptionString("value", "Description").WithMaxLength(6001), panics: true},
		{
			name:   "min length greater than max length",
			option: rave.OptionString("value", "Description").WithMinLength(2).WithMaxLength(1),
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := func() {
				rave.Slash("root", "Description").AddOptions(tt.option).Build()
			}
			if tt.panics {
				require.Panics(t, build)
				return
			}
			require.NotPanics(t, build)
		})
	}
}

func buildSlashOption(option rave.CommandOption) {
	rave.Slash("root", "Description").AddOptions(option).Build()
}

func TestStringOptionChoiceNameAndValueBounds(t *testing.T) {
	tests := []struct {
		name   string
		build  func()
		panics bool
	}{
		{
			name:  "AddChoice accepts one-rune name and empty value",
			build: func() { buildSlashOption(rave.OptionString("value", "Description").AddChoice("å", "")) },
		},
		{
			name: "AddChoice accepts 100-rune name and value",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").
					AddChoice(strings.Repeat("å", 100), strings.Repeat("界", 100)))
			},
		},
		{
			name:   "AddChoice rejects empty name",
			build:  func() { buildSlashOption(rave.OptionString("value", "Description").AddChoice("", "value")) },
			panics: true,
		},
		{
			name: "AddChoice rejects 101-rune name",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").
					AddChoice(strings.Repeat("å", 101), "value"))
			},
			panics: true,
		},
		{
			name: "AddChoice rejects invalid UTF-8 name",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").
					AddChoice(string([]byte{0xff}), "value"))
			},
			panics: true,
		},
		{
			name: "AddChoice rejects 101-rune value",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").
					AddChoice("Choice", strings.Repeat("界", 101)))
			},
			panics: true,
		},
		{
			name: "AddChoice rejects invalid UTF-8 value",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").
					AddChoice("Choice", string([]byte{0xff})))
			},
			panics: true,
		},
		{
			name: "WithChoices accepts empty and 100-rune values",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{
						{Name: "Empty", Value: ""},
						{Name: "Maximum", Value: strings.Repeat("界", 100)},
					},
				))
			},
		},
		{
			name: "WithChoices rejects 101-rune value",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:  "Choice",
						Value: strings.Repeat("界", 101),
					}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects invalid UTF-8 value",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:  "Choice",
						Value: string([]byte{0xff}),
					}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices accepts one-rune localization",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: "å"},
						Value:             "value",
					}},
				))
			},
		},
		{
			name: "WithChoices accepts 100-rune localization",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: strings.Repeat("界", 100)},
						Value:             "value",
					}},
				))
			},
		},
		{
			name: "WithChoices rejects empty localization",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: ""},
						Value:             "value",
					}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects 101-rune localization",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: strings.Repeat("界", 101)},
						Value:             "value",
					}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects invalid UTF-8 localization",
			build: func() {
				buildSlashOption(rave.OptionString("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceString{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: string([]byte{0xff})},
						Value:             "value",
					}},
				))
			},
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				require.Panics(t, tt.build)
				return
			}
			require.NotPanics(t, tt.build)
		})
	}
}

func TestIntegerOptionChoiceBounds(t *testing.T) {
	const maxSafeInteger = 1<<53 - 1

	tests := []struct {
		name   string
		build  func()
		panics bool
	}{
		{
			name:  "AddChoice accepts minimum safe integer",
			build: func() { buildSlashOption(rave.OptionInt("value", "Description").AddChoice("Minimum", -maxSafeInteger)) },
		},
		{
			name:  "AddChoice accepts maximum safe integer",
			build: func() { buildSlashOption(rave.OptionInt("value", "Description").AddChoice("Maximum", maxSafeInteger)) },
		},
		{
			name:   "AddChoice rejects below minimum safe integer",
			build:  func() { buildSlashOption(rave.OptionInt("value", "Description").AddChoice("Below", -maxSafeInteger-1)) },
			panics: true,
		},
		{
			name:   "AddChoice rejects above maximum safe integer",
			build:  func() { buildSlashOption(rave.OptionInt("value", "Description").AddChoice("Above", maxSafeInteger+1)) },
			panics: true,
		},
		{
			name: "WithChoices accepts safe integer bounds",
			build: func() {
				buildSlashOption(rave.OptionInt("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceInt{
						{Name: "Minimum", Value: -maxSafeInteger},
						{Name: "Maximum", Value: maxSafeInteger},
					},
				))
			},
		},
		{
			name: "WithChoices rejects below minimum safe integer",
			build: func() {
				buildSlashOption(rave.OptionInt("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceInt{{Name: "Below", Value: -maxSafeInteger - 1}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects above maximum safe integer",
			build: func() {
				buildSlashOption(rave.OptionInt("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceInt{{Name: "Above", Value: maxSafeInteger + 1}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects 101-rune name",
			build: func() {
				buildSlashOption(rave.OptionInt("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceInt{{Name: strings.Repeat("å", 101), Value: 0}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects 101-rune localization",
			build: func() {
				buildSlashOption(rave.OptionInt("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceInt{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: strings.Repeat("界", 101)},
						Value:             0,
					}},
				))
			},
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				require.Panics(t, tt.build)
				return
			}
			require.NotPanics(t, tt.build)
		})
	}
}

func TestIntegerOptionValueBounds(t *testing.T) {
	const maxSafeInteger = 1<<53 - 1

	tests := []struct {
		name   string
		option rave.CommandOption
		panics bool
	}{
		{
			name:   "accepts exact bounds",
			option: rave.OptionInt("value", "Description").WithMinValue(-maxSafeInteger).WithMaxValue(maxSafeInteger),
		},
		{
			name:   "rejects minimum below safe range",
			option: rave.OptionInt("value", "Description").WithMinValue(-maxSafeInteger - 1),
			panics: true,
		},
		{
			name:   "rejects maximum above safe range",
			option: rave.OptionInt("value", "Description").WithMaxValue(maxSafeInteger + 1),
			panics: true,
		},
		{
			name:   "rejects minimum above safe range",
			option: rave.OptionInt("value", "Description").WithMinValue(maxSafeInteger + 1),
			panics: true,
		},
		{
			name:   "rejects maximum below safe range",
			option: rave.OptionInt("value", "Description").WithMaxValue(-maxSafeInteger - 1),
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				require.Panics(t, func() { buildSlashOption(tt.option) })
				return
			}
			require.NotPanics(t, func() { buildSlashOption(tt.option) })
		})
	}
}

func TestNumberOptionChoiceBounds(t *testing.T) {
	const maxNumber = float64(1 << 53)

	tests := []struct {
		name   string
		build  func()
		panics bool
	}{
		{
			name:  "AddChoice accepts minimum number",
			build: func() { buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("Minimum", -maxNumber)) },
		},
		{
			name:  "AddChoice accepts maximum number",
			build: func() { buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("Maximum", maxNumber)) },
		},
		{
			name: "AddChoice rejects next number below minimum",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("Below", math.Nextafter(-maxNumber, math.Inf(-1))))
			},
			panics: true,
		},
		{
			name: "AddChoice rejects next number above maximum",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("Above", math.Nextafter(maxNumber, math.Inf(1))))
			},
			panics: true,
		},
		{
			name:   "AddChoice rejects NaN",
			build:  func() { buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("NaN", math.NaN())) },
			panics: true,
		},
		{
			name:   "AddChoice rejects positive infinity",
			build:  func() { buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("Infinity", math.Inf(1))) },
			panics: true,
		},
		{
			name:   "AddChoice rejects negative infinity",
			build:  func() { buildSlashOption(rave.OptionFloat("value", "Description").AddChoice("Infinity", math.Inf(-1))) },
			panics: true,
		},
		{
			name: "WithChoices accepts number bounds",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{
						{Name: "Minimum", Value: -maxNumber},
						{Name: "Maximum", Value: maxNumber},
					},
				))
			},
		},
		{
			name: "WithChoices rejects next number below minimum",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{Name: "Below", Value: math.Nextafter(-maxNumber, math.Inf(-1))}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects next number above maximum",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{Name: "Above", Value: math.Nextafter(maxNumber, math.Inf(1))}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects NaN",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{Name: "NaN", Value: math.NaN()}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects positive infinity",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{Name: "Infinity", Value: math.Inf(1)}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects negative infinity",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{Name: "Infinity", Value: math.Inf(-1)}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects 101-rune name",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{Name: strings.Repeat("å", 101), Value: 0}},
				))
			},
			panics: true,
		},
		{
			name: "WithChoices rejects 101-rune localization",
			build: func() {
				buildSlashOption(rave.OptionFloat("value", "Description").WithChoices(
					[]discord.ApplicationCommandOptionChoiceFloat{{
						Name:              "Choice",
						NameLocalizations: map[discord.Locale]string{discord.LocaleNorwegian: strings.Repeat("界", 101)},
						Value:             0,
					}},
				))
			},
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				require.Panics(t, tt.build)
				return
			}
			require.NotPanics(t, tt.build)
		})
	}
}

func TestNumberOptionValueBounds(t *testing.T) {
	const maxNumber = float64(1 << 53)

	tests := []struct {
		name   string
		option rave.CommandOption
		panics bool
	}{
		{
			name:   "accepts exact bounds",
			option: rave.OptionFloat("value", "Description").WithMinValue(-maxNumber).WithMaxValue(maxNumber),
		},
		{
			name:   "rejects minimum below range",
			option: rave.OptionFloat("value", "Description").WithMinValue(math.Nextafter(-maxNumber, math.Inf(-1))),
			panics: true,
		},
		{
			name:   "rejects maximum above range",
			option: rave.OptionFloat("value", "Description").WithMaxValue(math.Nextafter(maxNumber, math.Inf(1))),
			panics: true,
		},
		{
			name:   "rejects minimum above range",
			option: rave.OptionFloat("value", "Description").WithMinValue(math.Nextafter(maxNumber, math.Inf(1))),
			panics: true,
		},
		{
			name:   "rejects maximum below range",
			option: rave.OptionFloat("value", "Description").WithMaxValue(math.Nextafter(-maxNumber, math.Inf(-1))),
			panics: true,
		},
		{
			name:   "rejects NaN minimum",
			option: rave.OptionFloat("value", "Description").WithMinValue(math.NaN()),
			panics: true,
		},
		{
			name:   "rejects NaN maximum",
			option: rave.OptionFloat("value", "Description").WithMaxValue(math.NaN()),
			panics: true,
		},
		{
			name:   "rejects positive infinity minimum",
			option: rave.OptionFloat("value", "Description").WithMinValue(math.Inf(1)),
			panics: true,
		},
		{
			name:   "rejects negative infinity maximum",
			option: rave.OptionFloat("value", "Description").WithMaxValue(math.Inf(-1)),
			panics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				require.Panics(t, func() { buildSlashOption(tt.option) })
				return
			}
			require.NotPanics(t, func() { buildSlashOption(tt.option) })
		})
	}
}

func TestStaticChoiceCountBounds(t *testing.T) {
	stringOption := func(count int) rave.CommandOption {
		option := rave.OptionString("value", "Description")
		for i := range count {
			option.AddChoice(fmt.Sprintf("Choice %d", i), fmt.Sprintf("value%d", i))
		}
		return option
	}
	integerOption := func(count int) rave.CommandOption {
		choices := make([]discord.ApplicationCommandOptionChoiceInt, count)
		for i := range choices {
			choices[i] = discord.ApplicationCommandOptionChoiceInt{Name: fmt.Sprintf("Choice %d", i), Value: i}
		}
		return rave.OptionInt("value", "Description").WithChoices(choices)
	}
	floatOption := func(count int) rave.CommandOption {
		choices := make([]discord.ApplicationCommandOptionChoiceFloat, count)
		for i := range choices {
			choices[i] = discord.ApplicationCommandOptionChoiceFloat{Name: fmt.Sprintf("Choice %d", i), Value: float64(i)}
		}
		return rave.OptionFloat("value", "Description").WithChoices(choices)
	}

	require.NotPanics(t, func() { buildSlashOption(stringOption(25)) })
	require.Panics(t, func() { buildSlashOption(stringOption(26)) })
	require.NotPanics(t, func() { buildSlashOption(integerOption(25)) })
	require.Panics(t, func() { buildSlashOption(integerOption(26)) })
	require.NotPanics(t, func() { buildSlashOption(floatOption(25)) })
	require.Panics(t, func() { buildSlashOption(floatOption(26)) })
}

func TestDiscordNameRuleAcceptsAllowedUnicodeAndPunctuation(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{
			name:  "root apostrophe",
			build: func() { rave.Slash("can't", "Description").Build() },
		},
		{
			name:  "root Chinese letters",
			build: func() { rave.Slash("命令", "Description").Build() },
		},
		{
			name: "localization letter-number and other-number runes",
			build: func() {
				rave.Slash("root", "Description").
					AddNameLocalization(discord.LocaleChineseCN, "ⅷ²").
					Build()
			},
		},
		{
			name: "localization Devanagari mark",
			build: func() {
				rave.Slash("root", "Description").
					AddNameLocalization(discord.LocaleHindi, "का").
					Build()
			},
		},
		{
			name: "option Thai mark",
			build: func() {
				buildSlashOption(rave.OptionString("ก้", "Description"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, tt.build)
		})
	}
}

func TestDiscordNameRuleRejectsRunesWithLowercaseMappings(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{
			name:  "uppercase root",
			build: func() { rave.Slash("Root", "Description").Build() },
		},
		{
			name: "titlecase localization",
			build: func() {
				rave.Slash("root", "Description").
					AddNameLocalization(discord.LocaleNorwegian, "ǅ").
					Build()
			},
		},
		{
			name: "uppercase letter-number option",
			build: func() {
				buildSlashOption(rave.OptionString("Ⅷ", "Description"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Panics(t, tt.build)
		})
	}
}

func TestSlashCommandBuildRejectsInvalidRootDefinition(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		description string
	}{
		{name: "invalid name", commandName: "Not-Lowercase", description: "Description"},
		{name: "empty description", commandName: "valid", description: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := rave.Slash(tt.commandName, tt.description)

			require.Panics(t, func() {
				command.Build()
			})
		})
	}
}

func TestSlashCommandNameLengthCountsUnicodeCharacters(t *testing.T) {
	require.NotPanics(t, func() {
		rave.Slash(strings.Repeat("å", 32), "Description").Build()
	})
	require.Panics(t, func() {
		rave.Slash(strings.Repeat("å", 33), "Description").Build()
	})
}

func TestSlashCommandDescriptionLengthCountsUnicodeCharacters(t *testing.T) {
	require.NotPanics(t, func() {
		rave.Slash("valid", strings.Repeat("å", 100)).Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", strings.Repeat("å", 101)).Build()
	})
}

func TestSlashCommandLocalizationLengthsCountUnicodeCharacters(t *testing.T) {
	require.NotPanics(t, func() {
		rave.Slash("valid", "Description").
			AddNameLocalization(discord.LocaleNorwegian, strings.Repeat("å", 32)).
			AddDescriptionLocalization(discord.LocaleNorwegian, strings.Repeat("å", 100)).
			Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", "Description").
			AddNameLocalization(discord.LocaleNorwegian, strings.Repeat("å", 33)).
			Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", "Description").
			AddDescriptionLocalization(discord.LocaleNorwegian, strings.Repeat("å", 101)).
			Build()
	})
}

func TestSlashCommandOptionLengthsCountUnicodeCharacters(t *testing.T) {
	require.NotPanics(t, func() {
		rave.Slash("valid", "Description").AddOptions(
			rave.OptionString(strings.Repeat("å", 32), strings.Repeat("å", 100)).
				AddNameLocalization(discord.LocaleNorwegian, strings.Repeat("å", 32)).
				AddDescriptionLocalization(discord.LocaleNorwegian, strings.Repeat("å", 100)),
		).Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", "Description").AddOptions(
			rave.OptionString(strings.Repeat("å", 33), "Description"),
		).Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", "Description").AddOptions(
			rave.OptionString("valid", "Description").
				AddNameLocalization(discord.LocaleNorwegian, strings.Repeat("å", 33)),
		).Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", "Description").AddOptions(
			rave.OptionString("valid", strings.Repeat("å", 101)),
		).Build()
	})
	require.Panics(t, func() {
		rave.Slash("valid", "Description").AddOptions(
			rave.OptionString("valid", "Description").
				AddDescriptionLocalization(discord.LocaleNorwegian, strings.Repeat("å", 101)),
		).Build()
	})
}

func TestSubcommandsDoNotExposeRequiredOptionMethod(t *testing.T) {
	_, subCommandHasRequired := reflect.TypeOf(
		rave.SubCommand("child", "Child command"),
	).MethodByName("WithRequired")
	_, groupHasRequired := reflect.TypeOf(
		rave.SubCommandGroup("group", "Command group"),
	).MethodByName("WithRequired")

	require.False(t, subCommandHasRequired)
	require.False(t, groupHasRequired)
}

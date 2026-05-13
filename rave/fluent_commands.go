package rave

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

var (
	ErrCommandNameInvalid = errors.New("invalid command/option name")
)

var commandNameAndOptionRegex = regexp.MustCompile(`^[-_'\p{L}\p{N}\p{Devanagari}\p{Thai}]{1,32}$`)

func validateCommandOrOptionName(name string) (ok bool, err error) {
	if strings.ToLower(name) != name {
		return false, fmt.Errorf("%w: must be lowercase", ErrCommandNameInvalid)
	}

	length := utf8.RuneCountInString(name)
	if length < 1 || length > 32 {
		return false, fmt.Errorf("%w: must be 1-32 characters long", ErrCommandNameInvalid)
	}

	if !commandNameAndOptionRegex.MatchString(name) {
		return false, ErrCommandNameInvalid
	}

	return true, nil
}

type SlashCommand struct {
	discord.SlashCommandCreate

	Handlers map[string]func(e *handler.CommandEvent) error
}

type SubCommand struct {
	Name string
}

func NewSlashCommand(name, description string) *SlashCommand {
	sc := new(SlashCommand)
	sc.Name = name
	sc.Description = description
	return sc
}

func (sc *SlashCommand) WithLocalizedName(locale discord.Locale, name string) *SlashCommand {
	sc.NameLocalizations[locale] = name
	return sc
}

func (sc *SlashCommand) WithLocalizedDescription(locale discord.Locale, description string) *SlashCommand {
	sc.DescriptionLocalizations[locale] = description
	return sc
}

func Foo(a, b, c string) {
	println(a, b, c)
}

func Bar() (a, b, c string) {
	return "foo", "bar", "baz"
}

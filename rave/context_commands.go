package rave

import (
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
)

type contextCommandBase[T any] struct {
	commandMetadataBase[T]
	self              *T
	name              string
	nameLocalizations map[discord.Locale]string
	handler           handler.CommandHandler
}

func newContextCommandBase[T any](self *T, name string) contextCommandBase[T] {
	return contextCommandBase[T]{
		commandMetadataBase: newCommandMetadataBase(self),
		self:                self,
		name:                name,
	}
}

func (c *contextCommandBase[T]) WithNameLocalizations(localizations map[discord.Locale]string) *T {
	c.nameLocalizations = localizations
	return c.self
}

func (c *contextCommandBase[T]) AddNameLocalization(locale discord.Locale, name string) *T {
	if c.nameLocalizations == nil {
		c.nameLocalizations = make(map[discord.Locale]string)
	}
	c.nameLocalizations[locale] = name
	return c.self
}

func (c *contextCommandBase[T]) Handle(h handler.CommandHandler) *T {
	c.handler = h
	return c.self
}

func (c *contextCommandBase[T]) validateName() {
	if !validContextCommandName(c.name) {
		panic("invalid Discord context command name: " + c.name)
	}
	for _, localizedName := range c.nameLocalizations {
		if !validContextCommandName(localizedName) {
			panic("invalid Discord context command name localization: " + localizedName)
		}
	}
}

func validContextCommandName(name string) bool {
	if !utf8.ValidString(name) {
		return false
	}
	length := utf8.RuneCountInString(name)
	return length >= 1 && length <= 32
}

type UserCommandBuilder struct {
	contextCommandBase[UserCommandBuilder]
	userHandler handler.UserCommandHandler
}

func UserCommand(name string) *UserCommandBuilder {
	c := &UserCommandBuilder{}
	c.contextCommandBase = newContextCommandBase(c, name)
	return c
}

func (c *UserCommandBuilder) Handle(h handler.CommandHandler) *UserCommandBuilder {
	rejectMixedHandlerStyles(h != nil, c.userHandler != nil, "user command")
	c.handler = h
	return c
}

func (c *UserCommandBuilder) HandleUser(h handler.UserCommandHandler) *UserCommandBuilder {
	rejectMixedHandlerStyles(h != nil, c.handler != nil, "user command")
	c.userHandler = h
	return c
}

func (c *UserCommandBuilder) Build() discord.ApplicationCommandCreate {
	c.validateName()
	return discord.UserCommandCreate{
		Name:                     c.name,
		NameLocalizations:        c.nameLocalizations,
		DefaultMemberPermissions: omit.New(c.defaultMemberPermissions),
		IntegrationTypes:         c.integrationTypes,
		Contexts:                 c.contexts,
		NSFW:                     c.nsfw,
	}
}

type MessageCommandBuilder struct {
	contextCommandBase[MessageCommandBuilder]
	messageHandler handler.MessageCommandHandler
}

func MessageCommand(name string) *MessageCommandBuilder {
	c := &MessageCommandBuilder{}
	c.contextCommandBase = newContextCommandBase(c, name)
	return c
}

func (c *MessageCommandBuilder) Handle(h handler.CommandHandler) *MessageCommandBuilder {
	rejectMixedHandlerStyles(h != nil, c.messageHandler != nil, "message command")
	c.handler = h
	return c
}

func (c *MessageCommandBuilder) HandleMessage(h handler.MessageCommandHandler) *MessageCommandBuilder {
	rejectMixedHandlerStyles(h != nil, c.handler != nil, "message command")
	c.messageHandler = h
	return c
}

func (c *MessageCommandBuilder) Build() discord.ApplicationCommandCreate {
	c.validateName()
	return discord.MessageCommandCreate{
		Name:                     c.name,
		NameLocalizations:        c.nameLocalizations,
		DefaultMemberPermissions: omit.New(c.defaultMemberPermissions),
		IntegrationTypes:         c.integrationTypes,
		Contexts:                 c.contexts,
		NSFW:                     c.nsfw,
	}
}

package rave

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

func (c *UserCommandBuilder) Register(router handler.Router) discord.ApplicationCommandCreate {
	built := c.Build()
	path := "/" + c.name
	if c.userHandler != nil {
		router.UserCommand(path, c.userHandler)
	} else if c.handler != nil {
		router.Command(path, c.handler)
	} else {
		panic("executable Discord command is missing a handler: " + path)
	}
	return built
}

func (c *MessageCommandBuilder) Register(router handler.Router) discord.ApplicationCommandCreate {
	built := c.Build()
	path := "/" + c.name
	if c.messageHandler != nil {
		router.MessageCommand(path, c.messageHandler)
	} else if c.handler != nil {
		router.Command(path, c.handler)
	} else {
		panic("executable Discord command is missing a handler: " + path)
	}
	return built
}

// Register installs every executable command route on router and returns the
// Discord command payload built from the same command tree.
func (s *SlashCommandBuilder) Register(router handler.Router) discord.ApplicationCommandCreate {
	built := s.Build()
	path := "/" + s.name

	if hasNestedOptions(s.options) {
		if s.handler != nil || s.slashHandler != nil {
			panic("Discord command containers cannot have handlers: " + path)
		}
		s.registerChildren(router, path)
		return built
	}

	registerExecutable(router, path, s.handler, s.slashHandler, s.options)
	return built
}

func (s *SlashCommandBuilder) registerChildren(router handler.Router, parentPath string) {
	for _, option := range s.options {
		switch option := option.(type) {
		case *optionSubCommand:
			option.register(router, parentPath+"/"+option.name)
		case *optionSubCommandGroup:
			option.register(router, parentPath+"/"+option.name)
		default:
			panic("Discord command cannot mix primitive options and subcommands")
		}
	}
}

func (s *optionSubCommand) register(router handler.Router, path string) {
	registerExecutable(router, path, s.handler, s.slashHandler, s.options)
}

func (g *optionSubCommandGroup) register(router handler.Router, path string) {
	for _, subCommand := range g.options {
		subCommand.register(router, path+"/"+subCommand.name)
	}
}

func registerExecutable(
	router handler.Router,
	path string,
	h handler.CommandHandler,
	slashHandler handler.SlashCommandHandler,
	options []CommandOption,
) {
	if slashHandler != nil {
		router.SlashCommand(path, slashHandler)
	} else if h != nil {
		router.Command(path, h)
	} else {
		panic("executable Discord command is missing a handler: " + path)
	}

	autocompleteHandlers := collectAutocompleteHandlers(options)
	if len(autocompleteHandlers) == 0 {
		return
	}
	router.Autocomplete(path, dispatchAutocomplete(autocompleteHandlers))
}

func collectAutocompleteHandlers(options []CommandOption) map[string]internalAutocompleteHandler {
	handlers := make(map[string]internalAutocompleteHandler)
	for _, option := range options {
		provider, ok := option.(autocompleteProvider)
		if !ok {
			continue
		}
		name, h, configured := provider.autocompleteBinding()
		if configured {
			handlers[name] = h
		}
	}
	return handlers
}

func dispatchAutocomplete(
	handlers map[string]internalAutocompleteHandler,
) handler.AutocompleteHandler {
	return func(e *handler.AutocompleteEvent) error {
		focused := e.Data.Focused()
		h, ok := handlers[focused.Name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrAutocompleteHandlerNotFound, focused.Name)
		}
		return h(e, focused)
	}
}

func hasNestedOptions(options []CommandOption) bool {
	for _, option := range options {
		switch option.(type) {
		case *optionSubCommand, *optionSubCommandGroup:
			return true
		}
	}
	return false
}

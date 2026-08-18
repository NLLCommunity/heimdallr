package rave

import (
	"errors"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
)

type registerMaybe interface {
	register(r handler.Router) (command discord.ApplicationCommandCreate, hasCommand bool)
}

type BundledInteractions func(r handler.Router) []discord.ApplicationCommandCreate

func Bundle(items ...registerMaybe) BundledInteractions {
	return func(r handler.Router) []discord.ApplicationCommandCreate {
		var commands []discord.ApplicationCommandCreate
		for _, item := range items {
			if cmd, has := item.register(r); has {
				commands = append(commands, cmd)
			}
		}
		return commands
	}
}

type RaveClient struct {
	*bot.Client
	Router handler.Router
}

func NewClient(token string, opts ...bot.ConfigOpt) (*RaveClient, error) {
	return NewClientWithRouter(token, handler.New(), opts...)
}

func NewClientWithRouter(token string, router handler.Router, opts ...bot.ConfigOpt) (*RaveClient, error) {
	if router == nil {
		return nil, errors.New("router must not be nil")
	}

	disgoClient, err := disgo.New(token, opts...)
	if err != nil {
		return nil, err
	}

	disgoClient.AddEventListeners(router)
	return new(RaveClient{Client: disgoClient, Router: router}), nil
}

func (c *RaveClient) RegisterAndSyncBundlesGlobal(interactions ...BundledInteractions) error {
	return c.RegisterAndSyncBundles(nil, interactions...)
}

func (c *RaveClient) RegisterAndSyncBundles(guilds []snowflake.ID, interactions ...BundledInteractions) error {
	var commandCreates []discord.ApplicationCommandCreate
	for _, register := range interactions {
		commandCreates = append(commandCreates, register(c.Router)...)
	}

	return handler.SyncCommands(c.Client, commandCreates, guilds)
}

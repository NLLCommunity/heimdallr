package rave

import (
	"errors"
	"sync"

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

var (
	ErrBundlesAlreadyRegistered = errors.New("bundles already registered")
	ErrBundlesNotRegistered     = errors.New("bundles not registered")
)

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

	bundleMu          sync.Mutex
	bundlesInstalled  bool
	installedGuilds   []snowflake.ID
	installedCommands []discord.ApplicationCommandCreate
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

// RegisterAndSyncBundles installs and synchronizes the supplied bundles once.
// Later registration calls fail even when the initial synchronization fails;
// RetrySyncBundles preserves the initial synchronization scope.
func (c *RaveClient) RegisterAndSyncBundles(guilds []snowflake.ID, interactions ...BundledInteractions) error {
	c.bundleMu.Lock()
	defer c.bundleMu.Unlock()

	if c.bundlesInstalled {
		return ErrBundlesAlreadyRegistered
	}

	for _, register := range interactions {
		c.installedCommands = append(c.installedCommands, register(c.Router)...)
	}
	c.installedGuilds = append([]snowflake.ID(nil), guilds...)
	c.bundlesInstalled = true

	return handler.SyncCommands(c.Client, c.installedCommands, c.installedGuilds)
}

// RetrySyncBundles synchronizes the bundles registered by the first
// RegisterAndSyncBundles call using the initial synchronization scope.
func (c *RaveClient) RetrySyncBundles() error {
	c.bundleMu.Lock()
	defer c.bundleMu.Unlock()

	if !c.bundlesInstalled {
		return ErrBundlesNotRegistered
	}

	return handler.SyncCommands(c.Client, c.installedCommands, c.installedGuilds)
}

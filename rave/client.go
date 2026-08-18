package rave

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

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

	bundleMu             sync.Mutex
	bundlesInstalled     bool
	installedBundleCount int
	installedBundles     []BundledInteractions
	installedCommands    []discord.ApplicationCommandCreate
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

// RegisterAndSyncBundles installs and synchronizes the supplied bundles. Retry
// calls must reuse the exact same stored function values in the same order;
// freshly evaluated equivalent closures are rejected.
func (c *RaveClient) RegisterAndSyncBundles(guilds []snowflake.ID, interactions ...BundledInteractions) error {
	c.bundleMu.Lock()
	defer c.bundleMu.Unlock()

	if !c.bundlesInstalled {
		for _, register := range interactions {
			c.installedCommands = append(c.installedCommands, register(c.Router)...)
		}
		c.installedBundleCount = len(interactions)
		c.installedBundles = append([]BundledInteractions(nil), interactions...)
		c.bundlesInstalled = true
	} else if len(interactions) != c.installedBundleCount {
		return fmt.Errorf(
			"bundle installation already initialized with %d bundles; got %d",
			c.installedBundleCount,
			len(interactions),
		)
	} else if !sameBundleIdentities(c.installedBundles, interactions) {
		return errors.New("bundle installation already initialized with different bundles")
	}

	return handler.SyncCommands(c.Client, c.installedCommands, guilds)
}

type bundledInteractionIdentity struct {
	entry uintptr
	value uintptr
}

func sameBundleIdentities(installed, bundles []BundledInteractions) bool {
	if len(installed) != len(bundles) {
		return false
	}
	for i, bundle := range bundles {
		if bundleIdentity(installed[i]) != bundleIdentity(bundle) {
			return false
		}
	}
	return true
}

func bundleIdentity(bundle BundledInteractions) bundledInteractionIdentity {
	return bundledInteractionIdentity{
		entry: reflect.ValueOf(bundle).Pointer(),
		value: functionValuePointer(bundle),
	}
}

// functionValuePointer returns the runtime func-value allocation backing bundle.
// Go does not guarantee this representation, so this process-local identity is
// intentionally isolated and paired with the documented function entry pointer.
func functionValuePointer(bundle BundledInteractions) uintptr {
	return uintptr(*(*unsafe.Pointer)(unsafe.Pointer(&bundle)))
}

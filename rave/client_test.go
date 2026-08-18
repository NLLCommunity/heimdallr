package rave_test

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/httpserver"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"

	"github.com/NLLCommunity/heimdallr/rave"
)

type recordedRESTRequest struct {
	method   string
	url      string
	commands []discord.ApplicationCommandCreate
}

type recordingRESTClient struct {
	mu           sync.Mutex
	requests     []recordedRESTRequest
	queuedErrors []error
	delay        time.Duration
	inFlight     atomic.Int32
	maxInFlight  atomic.Int32
}

type recordingEventManager struct {
	listeners []bot.EventListener
}

func (m *recordingEventManager) AddEventListeners(listeners ...bot.EventListener) {
	m.listeners = append(m.listeners, listeners...)
}

func (m *recordingEventManager) RemoveEventListeners(listeners ...bot.EventListener) {
	for _, listener := range listeners {
		for i, registered := range m.listeners {
			if registered == listener {
				m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
				break
			}
		}
	}
}

func (*recordingEventManager) HandleGatewayEvent(
	gateway.Gateway,
	gateway.EventType,
	int,
	gateway.EventData,
) {
}

func (*recordingEventManager) HandleHTTPEvent(
	httpserver.RespondFunc,
	httpserver.EventInteractionCreate,
) {
}

func (m *recordingEventManager) DispatchEvent(event bot.Event) {
	for _, listener := range m.listeners {
		listener.OnEvent(event)
	}
}

func (c *recordingRESTClient) HTTPClient() *http.Client {
	return nil
}

func (c *recordingRESTClient) RateLimiter() rest.RateLimiter {
	return nil
}

func (c *recordingRESTClient) Close(context.Context) {}

func (c *recordingRESTClient) Do(
	endpoint *rest.CompiledEndpoint,
	input any,
	_ any,
	_ ...rest.RequestOpt,
) error {
	inFlight := c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	for {
		maxInFlight := c.maxInFlight.Load()
		if inFlight <= maxInFlight || c.maxInFlight.CompareAndSwap(maxInFlight, inFlight) {
			break
		}
	}
	if c.delay > 0 {
		time.Sleep(c.delay)
	}

	commands, _ := input.([]discord.ApplicationCommandCreate)
	c.mu.Lock()
	defer c.mu.Unlock()

	c.requests = append(c.requests, recordedRESTRequest{
		method:   endpoint.Endpoint.Method,
		url:      endpoint.URL,
		commands: append([]discord.ApplicationCommandCreate(nil), commands...),
	})
	if len(c.queuedErrors) > 0 {
		err := c.queuedErrors[0]
		c.queuedErrors = c.queuedErrors[1:]
		return err
	}
	return nil
}

func newSyncTestClient(recorder *recordingRESTClient) *rave.RaveClient {
	return &rave.RaveClient{
		Client: &bot.Client{
			ApplicationID: 123,
			Rest:          rest.New(recorder),
		},
		Router: handler.New(),
	}
}

func emptyBundle(handler.Router) []discord.ApplicationCommandCreate {
	return nil
}

//go:noinline
func bundledCommand(name string) rave.BundledInteractions {
	return rave.Bundle(rave.Slash(name, "Command").Handle(noopCommandHandler))
}

//go:noinline
func capturedCommandBundle(name string) rave.BundledInteractions {
	return func(router handler.Router) []discord.ApplicationCommandCreate {
		return rave.Bundle(rave.Slash(name, "Command").Handle(noopCommandHandler))(router)
	}
}

type bundleLifetimeMarker struct{ _ byte }

//go:noinline
func finalizableCapturingBundle(name string, finalized chan<- struct{}) rave.BundledInteractions {
	marker := &bundleLifetimeMarker{}
	runtime.SetFinalizer(marker, func(*bundleLifetimeMarker) { close(finalized) })
	return func(router handler.Router) []discord.ApplicationCommandCreate {
		runtime.KeepAlive(marker)
		return rave.Bundle(rave.Slash(name, "Command").Handle(noopCommandHandler))(router)
	}
}

func bundledCommandInteraction(t *testing.T) discord.Interaction {
	t.Helper()

	interaction, err := discord.UnmarshalInteraction([]byte(`{
		"id": "1",
		"application_id": "123456789012345678",
		"type": 2,
		"token": "interaction-token",
		"version": 1,
		"data": {
			"id": "2",
			"name": "bundled",
			"type": 1
		}
	}`))
	require.NoError(t, err)
	return interaction
}

func TestRegisterAndSyncBundlesForwardsGuilds(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)

	err := client.RegisterAndSyncBundles(
		[]snowflake.ID{456, 789},
		emptyBundle,
	)

	require.NoError(t, err)
	require.Equal(t, []recordedRESTRequest{
		{method: http.MethodPut, url: "/applications/123/guilds/456/commands"},
		{method: http.MethodPut, url: "/applications/123/guilds/789/commands"},
	}, recorder.requests)
}

func TestRegisterAndSyncBundlesGlobalSyncsGlobally(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)

	err := client.RegisterAndSyncBundlesGlobal(emptyBundle)

	require.NoError(t, err)
	require.Equal(t, []recordedRESTRequest{
		{method: http.MethodPut, url: "/applications/123/commands"},
	}, recorder.requests)
}

func TestRegisterAndSyncBundlesRetryInstallsRoutesOnce(t *testing.T) {
	syncErr := errors.New("temporary sync failure")
	recorder := &recordingRESTClient{queuedErrors: []error{syncErr, nil}}
	client := newSyncTestClient(recorder)
	var installs atomic.Int32
	var handlerCalls atomic.Int32
	bundle := func(router handler.Router) []discord.ApplicationCommandCreate {
		installs.Add(1)
		return rave.Bundle(
			rave.Slash("bundled", "Bundled command").
				Handle(func(*handler.CommandEvent) error {
					handlerCalls.Add(1)
					return nil
				}),
		)(router)
	}

	err := client.RegisterAndSyncBundlesGlobal(bundle)
	require.ErrorIs(t, err, syncErr)
	require.NoError(t, client.RegisterAndSyncBundlesGlobal(bundle))

	client.Router.OnEvent(&events.InteractionCreate{
		GenericEvent: events.NewGenericEvent(client.Client, 0, 0),
		Interaction:  bundledCommandInteraction(t),
	})

	require.Equal(t, int32(1), installs.Load())
	require.Equal(t, int32(1), handlerCalls.Load())
	require.Len(t, recorder.requests, 2)
	require.Len(t, recorder.requests[0].commands, 1)
	command, ok := recorder.requests[0].commands[0].(discord.SlashCommandCreate)
	require.True(t, ok)
	require.Equal(t, "bundled", command.Name)
	require.Equal(t, "Bundled command", command.Description)
	require.Equal(t, recorder.requests[0].commands, recorder.requests[1].commands)
}

func TestRegisterAndSyncBundlesRejectsDifferentSameCountBundles(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	var firstInstalls atomic.Int32
	var replacementInstalls atomic.Int32
	first := func(handler.Router) []discord.ApplicationCommandCreate {
		firstInstalls.Add(1)
		return nil
	}
	replacement := func(handler.Router) []discord.ApplicationCommandCreate {
		replacementInstalls.Add(1)
		return nil
	}

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(first))
	err := client.RegisterAndSyncBundlesGlobal(replacement)

	require.ErrorContains(t, err, "different bundles")
	require.Equal(t, int32(1), firstInstalls.Load())
	require.Zero(t, replacementInstalls.Load())
	require.Len(t, recorder.requests, 1)
}

func TestRegisterAndSyncBundlesRejectsDifferentBundleClosures(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	first := bundledCommand("first")
	replacement := bundledCommand("replacement")

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(first))
	err := client.RegisterAndSyncBundlesGlobal(replacement)

	require.ErrorContains(t, err, "different bundles")
	require.False(t, client.Router.Match("/replacement", discord.InteractionTypeApplicationCommand, int(discord.ApplicationCommandTypeSlash)))
	require.Len(t, recorder.requests, 1)
}

func TestRegisterAndSyncBundlesRejectsDifferentCapturingClosures(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	first := capturedCommandBundle("first")
	replacement := capturedCommandBundle("replacement")

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(first))
	err := client.RegisterAndSyncBundlesGlobal(replacement)

	require.ErrorContains(t, err, "different bundles")
	require.False(t, client.Router.Match("/replacement", discord.InteractionTypeApplicationCommand, int(discord.ApplicationCommandTypeSlash)))
	require.Len(t, recorder.requests, 1)
}

func TestRegisterAndSyncBundlesRetryReusesStoredCapturingBundle(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	bundle := capturedCommandBundle("stored")

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(bundle))
	require.NoError(t, client.RegisterAndSyncBundlesGlobal(bundle))
	require.Len(t, recorder.requests, 2)
}

func TestRegisterAndSyncBundlesRetainsTemporaryCapturingBundle(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	finalized := make(chan struct{})
	installTemporaryCapturingBundle(t, client, finalized)

	for range 4 {
		pressure := make([][]byte, 256)
		for i := range pressure {
			pressure[i] = make([]byte, 64<<10)
		}
		runtime.KeepAlive(pressure)
		runtime.GC()
	}
	select {
	case <-finalized:
		t.Fatal("installed bundle closure was collected")
	case <-time.After(50 * time.Millisecond):
	}

	err := client.RegisterAndSyncBundlesGlobal(finalizableCapturingBundle("replacement", make(chan struct{})))

	require.ErrorContains(t, err, "different bundles")
	require.False(t, client.Router.Match("/replacement", discord.InteractionTypeApplicationCommand, int(discord.ApplicationCommandTypeSlash)))
	require.Len(t, recorder.requests, 1)
}

func installTemporaryCapturingBundle(t *testing.T, client *rave.RaveClient, finalized chan<- struct{}) {
	t.Helper()
	require.NoError(t, client.RegisterAndSyncBundlesGlobal(finalizableCapturingBundle("first", finalized)))
}

func TestRegisterAndSyncBundlesCopiesBundleSlice(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	bundles := []rave.BundledInteractions{capturedCommandBundle("first")}
	stored := bundles[0]

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(bundles...))
	bundles[0] = capturedCommandBundle("replacement")
	require.NoError(t, client.RegisterAndSyncBundlesGlobal(stored))
	require.Len(t, recorder.requests, 2)
}

func TestRegisterAndSyncBundlesRejectsReorderedBundles(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)
	first := func(handler.Router) []discord.ApplicationCommandCreate { return nil }
	second := func(handler.Router) []discord.ApplicationCommandCreate { return nil }

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(first, second))
	err := client.RegisterAndSyncBundlesGlobal(second, first)

	require.ErrorContains(t, err, "different bundles")
	require.Len(t, recorder.requests, 1)
}

func TestRegisterAndSyncBundlesConcurrentRetriesReuseInstallation(t *testing.T) {
	syncErr := errors.New("temporary sync failure")
	recorder := &recordingRESTClient{
		queuedErrors: []error{syncErr},
		delay:        10 * time.Millisecond,
	}
	client := newSyncTestClient(recorder)
	var installs atomic.Int32
	var handlerCalls atomic.Int32
	bundle := func(router handler.Router) []discord.ApplicationCommandCreate {
		installs.Add(1)
		return rave.Bundle(
			rave.Slash("bundled", "Bundled command").
				Handle(func(*handler.CommandEvent) error {
					handlerCalls.Add(1)
					return nil
				}),
		)(router)
	}

	require.ErrorIs(t, client.RegisterAndSyncBundlesGlobal(bundle), syncErr)

	const retries = 16
	start := make(chan struct{})
	errs := make(chan error, retries)
	var wg sync.WaitGroup
	for range retries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- client.RegisterAndSyncBundlesGlobal(bundle)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	client.Router.OnEvent(&events.InteractionCreate{
		GenericEvent: events.NewGenericEvent(client.Client, 0, 0),
		Interaction:  bundledCommandInteraction(t),
	})

	require.Equal(t, int32(1), installs.Load())
	require.Equal(t, int32(1), handlerCalls.Load())
	require.Len(t, recorder.requests, retries+1)
	require.Equal(t, int32(1), recorder.maxInFlight.Load())
}

func TestRegisterAndSyncBundlesRetryRejectsDifferentBundleCount(t *testing.T) {
	recorder := &recordingRESTClient{}
	client := newSyncTestClient(recorder)

	require.NoError(t, client.RegisterAndSyncBundlesGlobal(emptyBundle))
	err := client.RegisterAndSyncBundlesGlobal(emptyBundle, emptyBundle)

	require.ErrorContains(t, err, "bundle installation")
	require.Len(t, recorder.requests, 1)
}

func TestNewClientCreatesAndRegistersRouter(t *testing.T) {
	recorder := &recordingRESTClient{}
	eventManager := &recordingEventManager{}
	client, err := rave.NewClient(
		"MTIzNDU2Nzg5MDEyMzQ1Njc4",
		bot.WithRestClient(recorder),
		bot.WithEventManager(eventManager),
	)
	require.NoError(t, err)

	called := false
	err = client.RegisterAndSyncBundlesGlobal(
		rave.Bundle(
			rave.Slash("bundled", "Bundled command").
				Handle(func(*handler.CommandEvent) error {
					called = true
					return nil
				}),
		),
	)
	require.NoError(t, err)

	client.EventManager.DispatchEvent(&events.InteractionCreate{
		GenericEvent: events.NewGenericEvent(client.Client, 0, 0),
		Interaction:  bundledCommandInteraction(t),
	})

	require.True(t, called)
}

func TestNewClientUsesConfiguredRouterForBundledRoutes(t *testing.T) {
	recorder := &recordingRESTClient{}
	eventManager := &recordingEventManager{}
	router := handler.New()
	var calls []string
	router.Use(func(next handler.Handler) handler.Handler {
		return func(event *handler.InteractionEvent) error {
			calls = append(calls, "middleware before")
			err := next(event)
			calls = append(calls, "middleware after")
			return err
		}
	})

	client, err := rave.NewClientWithRouter(
		"MTIzNDU2Nzg5MDEyMzQ1Njc4",
		router,
		bot.WithRestClient(recorder),
		bot.WithEventManager(eventManager),
	)
	require.NoError(t, err)

	err = client.RegisterAndSyncBundlesGlobal(
		rave.Bundle(
			rave.Slash("bundled", "Bundled command").
				Handle(func(*handler.CommandEvent) error {
					calls = append(calls, "handler")
					return nil
				}),
		),
	)
	require.NoError(t, err)

	client.EventManager.DispatchEvent(&events.InteractionCreate{
		GenericEvent: events.NewGenericEvent(client.Client, 0, 0),
		Interaction:  bundledCommandInteraction(t),
	})

	require.Equal(t, []string{
		"middleware before",
		"handler",
		"middleware after",
	}, calls)
}

func TestNewClientWithRouterRejectsNilRouter(t *testing.T) {
	client, err := rave.NewClientWithRouter(
		"MTIzNDU2Nzg5MDEyMzQ1Njc4",
		nil,
		bot.WithRestClient(&recordingRESTClient{}),
	)

	require.Nil(t, client)
	require.Error(t, err)
}

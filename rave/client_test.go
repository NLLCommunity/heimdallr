package rave_test

import (
	"context"
	"net/http"
	"testing"

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
	method string
	url    string
}

type recordingRESTClient struct {
	requests []recordedRESTRequest
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
	_ any,
	_ any,
	_ ...rest.RequestOpt,
) error {
	c.requests = append(c.requests, recordedRESTRequest{
		method: endpoint.Endpoint.Method,
		url:    endpoint.URL,
	})
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

package timeout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
)

type recordedRESTCall struct {
	endpoint *rest.CompiledEndpoint
	body     any
	opts     int
}

type recordingTransport struct {
	request *http.Request
	body    []byte
}

func (t *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	t.request = request.Clone(request.Context())
	t.body = body

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    request,
	}, nil
}

type recordingRESTClient struct {
	delegate rest.Client
	calls    []recordedRESTCall
	err      error
}

func newRecordingRESTClient(t *testing.T) (*recordingRESTClient, *recordingTransport) {
	t.Helper()

	transport := &recordingTransport{}
	delegate := rest.NewClient(
		"test-token",
		rest.WithHTTPClient(&http.Client{Transport: transport}),
		rest.WithURL("https://discord.test"),
	)
	recorder := &recordingRESTClient{delegate: delegate}
	t.Cleanup(func() { recorder.Close(context.Background()) })
	return recorder, transport
}

func (c *recordingRESTClient) HTTPClient() *http.Client {
	return c.delegate.HTTPClient()
}

func (c *recordingRESTClient) RateLimiter() rest.RateLimiter {
	return c.delegate.RateLimiter()
}

func (c *recordingRESTClient) Close(ctx context.Context) {
	c.delegate.Close(ctx)
}

func (c *recordingRESTClient) Do(
	endpoint *rest.CompiledEndpoint,
	body any,
	response any,
	opts ...rest.RequestOpt,
) error {
	c.calls = append(c.calls, recordedRESTCall{endpoint: endpoint, body: body, opts: len(opts)})
	if c.err != nil {
		return c.err
	}
	return c.delegate.Do(endpoint, body, response, opts...)
}

type recordedInteractionResponse struct {
	calls        int
	responseType discord.InteractionResponseType
	message      discord.MessageCreate
	err          error
}

type timeoutEventFixture struct {
	invokerID          snowflake.ID
	invokerPermissions discord.Permissions
	invokerRoleIDs     []snowflake.ID
	targetID           snowflake.ID
	targetPermissions  discord.Permissions
	targetRoleIDs      []snowflake.ID
	guildOwnerID       snowflake.ID
	roles              []discord.Role
}

func eligibleTimeoutEventFixture() timeoutEventFixture {
	return timeoutEventFixture{
		invokerID:          100,
		invokerPermissions: discord.PermissionModerateMembers,
		invokerRoleIDs:     []snowflake.ID{101},
		targetID:           400,
		targetRoleIDs:      []snowflake.ID{401},
		guildOwnerID:       999,
		roles: []discord.Role{
			{ID: 101, GuildID: 200, Name: "Moderator", Position: 10},
			{ID: 401, GuildID: 200, Name: "Member", Position: 5},
		},
	}
}

func newTimeoutEvent(
	t *testing.T,
	recorder *recordingRESTClient,
	duration string,
	reason *string,
) (*handler.CommandEvent, *recordedInteractionResponse) {
	return newTimeoutEventWithFixture(t, recorder, duration, reason, eligibleTimeoutEventFixture())
}

func newTimeoutEventWithFixture(
	t *testing.T,
	recorder *recordingRESTClient,
	duration string,
	reason *string,
	fixture timeoutEventFixture,
) (*handler.CommandEvent, *recordedInteractionResponse) {
	t.Helper()

	reasonOption := ""
	if reason != nil {
		reasonOption = fmt.Sprintf(`,{"name":"reason","type":3,"value":%q}`, *reason)
	}
	invokerRoleIDs, err := json.Marshal(fixture.invokerRoleIDs)
	require.NoError(t, err)
	targetRoleIDs, err := json.Marshal(fixture.targetRoleIDs)
	require.NoError(t, err)

	interaction, err := discord.UnmarshalInteraction([]byte(fmt.Sprintf(`{
		"id":"175928847299117063",
		"application_id":"123456789012345678",
		"type":2,
		"token":"interaction-token",
		"version":1,
		"guild_id":"200",
		"channel":{"id":"201","type":0,"name":"general"},
		"member":{
			"user":{"id":"%d","username":"moderator","discriminator":"0"},
			"roles":%s,
			"permissions":"%d"
		},
		"data":{
			"id":"300",
			"name":"timeout",
			"type":1,
			"resolved":{
				"users":{"%d":{"id":"%d","username":"target","discriminator":"0"}},
				"members":{"%d":{"roles":%s,"permissions":"%d"}}
			},
			"options":[
				{"name":"user","type":6,"value":"%d"},
				{"name":"duration","type":3,"value":%q}%s
			]
		}
	}`,
		fixture.invokerID,
		invokerRoleIDs,
		fixture.invokerPermissions,
		fixture.targetID,
		fixture.targetID,
		fixture.targetID,
		targetRoleIDs,
		fixture.targetPermissions,
		fixture.targetID,
		duration,
		reasonOption,
	)))
	require.NoError(t, err)

	caches := cache.New(cache.WithCaches(cache.FlagGuilds, cache.FlagRoles))
	caches.AddGuild(discord.Guild{ID: 200, Name: "Test Guild", OwnerID: fixture.guildOwnerID})
	for _, role := range fixture.roles {
		caches.AddRole(role)
	}
	client := &bot.Client{Rest: rest.New(recorder), Caches: caches}
	response := &recordedInteractionResponse{}

	event := &handler.CommandEvent{
		ApplicationCommandInteractionCreate: &events.ApplicationCommandInteractionCreate{
			GenericEvent:                  events.NewGenericEvent(client, 0, 0),
			ApplicationCommandInteraction: interaction.(discord.ApplicationCommandInteraction),
			Respond: func(responseType discord.InteractionResponseType, data discord.InteractionResponseData, _ ...rest.RequestOpt) error {
				message, ok := data.(discord.MessageCreate)
				if !ok {
					return fmt.Errorf("unexpected interaction response data %T", data)
				}
				response.calls++
				response.responseType = responseType
				response.message = message
				return response.err
			},
		},
	}
	return event, response
}

func requireEphemeralResponse(t *testing.T, response *recordedInteractionResponse, content string) {
	t.Helper()
	require.Equal(t, 1, response.calls)
	require.Equal(t, discord.InteractionResponseTypeCreateMessage, response.responseType)
	require.Equal(t, content, response.message.Content)
	require.True(t, response.message.Flags.Has(discord.MessageFlagEphemeral))
}

func TestTimeoutHandlerRejectsUnauthorizedInvoker(t *testing.T) {
	recorder, _ := newRecordingRESTClient(t)
	fixture := eligibleTimeoutEventFixture()
	fixture.invokerPermissions = 0
	event, response := newTimeoutEventWithFixture(t, recorder, "1h", nil, fixture)

	require.NoError(t, TimeoutHandler(event))

	requireEphemeralResponse(t, response, "You cannot timeout this user.")
	require.Empty(t, recorder.calls)
}

func TestTimeoutHandlerRejectsIneligibleTargets(t *testing.T) {
	tests := []struct {
		name    string
		fixture timeoutEventFixture
	}{
		{
			name: "self",
			fixture: func() timeoutEventFixture {
				fixture := eligibleTimeoutEventFixture()
				fixture.targetID = 100
				return fixture
			}(),
		},
		{
			name: "guild owner",
			fixture: func() timeoutEventFixture {
				fixture := eligibleTimeoutEventFixture()
				fixture.targetID = 999
				return fixture
			}(),
		},
		{
			name: "administrator",
			fixture: func() timeoutEventFixture {
				fixture := eligibleTimeoutEventFixture()
				fixture.targetRoleIDs = []snowflake.ID{402}
				fixture.roles = append(fixture.roles, discord.Role{
					ID:          402,
					GuildID:     200,
					Name:        "Administrator",
					Position:    2,
					Permissions: discord.PermissionAdministrator,
				})
				return fixture
			}(),
		},
		{
			name: "equal highest role",
			fixture: func() timeoutEventFixture {
				fixture := eligibleTimeoutEventFixture()
				fixture.roles[1].Position = 10
				return fixture
			}(),
		},
		{
			name: "higher role",
			fixture: func() timeoutEventFixture {
				fixture := eligibleTimeoutEventFixture()
				fixture.roles[1].Position = 11
				return fixture
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder, _ := newRecordingRESTClient(t)
			event, response := newTimeoutEventWithFixture(t, recorder, "1h", nil, test.fixture)

			require.NoError(t, TimeoutHandler(event))

			requireEphemeralResponse(t, response, "You cannot timeout this user.")
			require.Empty(t, recorder.calls)
		})
	}
}

func TestTimeoutHandlerAllowsEligibleLowerRoleTarget(t *testing.T) {
	recorder, _ := newRecordingRESTClient(t)
	event, response := newTimeoutEvent(t, recorder, "1h", nil)

	require.NoError(t, TimeoutHandler(event))

	requireEphemeralResponse(t, response, "User target has been timed out for 1 hour.")
	require.Len(t, recorder.calls, 1)
}

func TestTimeoutHandlerAllowsGuildOwnerAboveRoleHierarchy(t *testing.T) {
	recorder, _ := newRecordingRESTClient(t)
	fixture := eligibleTimeoutEventFixture()
	fixture.invokerID = fixture.guildOwnerID
	fixture.roles[1].Position = 11
	event, response := newTimeoutEventWithFixture(t, recorder, "1h", nil, fixture)

	require.NoError(t, TimeoutHandler(event))

	requireEphemeralResponse(t, response, "User target has been timed out for 1 hour.")
	require.Len(t, recorder.calls, 1)
}

func TestTimeoutHandlerRejectsInvalidAndOutOfRangeDurations(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		want     string
	}{
		{
			name:     "invalid syntax",
			duration: "later",
			want:     "Invalid duration format. Please use the format: 3w2d1h4m28s",
		},
		{
			name:     "below one second",
			duration: "0s",
			want:     "Duration must be at least 1 second.",
		},
		{
			name:     "above twenty-eight days",
			duration: "28d1s",
			want:     "Duration must be less than 28 days.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder, _ := newRecordingRESTClient(t)
			event, response := newTimeoutEvent(t, recorder, test.duration, nil)

			require.NoError(t, TimeoutHandler(event))

			requireEphemeralResponse(t, response, test.want)
			require.Empty(t, recorder.calls)
		})
	}
}

func TestTimeoutHandlerRejectsOverflowWithoutREST(t *testing.T) {
	recorder, _ := newRecordingRESTClient(t)
	event, response := newTimeoutEvent(t, recorder, "281474976710657d", nil)

	require.NoError(t, TimeoutHandler(event))

	requireEphemeralResponse(t, response, "Invalid duration format. Please use the format: 3w2d1h4m28s")
	require.Empty(t, recorder.calls)
}

func TestTimeoutCommandLimitsReasonLength(t *testing.T) {
	command := Timeout.Build().(discord.SlashCommandCreate)

	var reason discord.ApplicationCommandOptionString
	found := false
	for _, option := range command.Options {
		stringOption, ok := option.(discord.ApplicationCommandOptionString)
		if ok && stringOption.Name == "reason" {
			reason = stringOption
			found = true
			break
		}
	}

	require.True(t, found)
	require.NotNil(t, reason.MaxLength)
	require.Equal(t, 512, *reason.MaxLength)
}

func TestTimeoutHandlerAcceptsReasonAtLimit(t *testing.T) {
	recorder, _ := newRecordingRESTClient(t)
	reason := strings.Repeat("å", 512)
	event, response := newTimeoutEvent(t, recorder, "1h", &reason)

	require.NoError(t, TimeoutHandler(event))

	requireEphemeralResponse(t, response, "User target has been timed out for 1 hour.")
	require.Len(t, recorder.calls, 1)
}

func TestTimeoutHandlerRejectsReasonOverLimit(t *testing.T) {
	recorder, _ := newRecordingRESTClient(t)
	reason := strings.Repeat("å", 513)
	event, response := newTimeoutEvent(t, recorder, "1h", &reason)

	require.NoError(t, TimeoutHandler(event))

	requireEphemeralResponse(t, response, "Reason must be 512 characters or fewer.")
	require.Empty(t, recorder.calls)
}

func TestTimeoutHandlerAcceptsBoundaryDurations(t *testing.T) {
	tests := []struct {
		name            string
		duration        string
		wantDuration    time.Duration
		reason          *string
		wantAuditReason string
		wantResponse    string
	}{
		{
			name:            "one second with default reason",
			duration:        "1s",
			wantDuration:    time.Second,
			wantAuditReason: "No reason provided.",
			wantResponse:    "User target has been timed out for 1 second.",
		},
		{
			name:            "twenty-eight days with explicit reason",
			duration:        "28d",
			wantDuration:    28 * 24 * time.Hour,
			reason:          ptr("Repeated spam / links"),
			wantAuditReason: "Repeated spam %2F links",
			wantResponse:    "User target has been timed out for 4 weeks.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder, transport := newRecordingRESTClient(t)
			event, response := newTimeoutEvent(t, recorder, test.duration, test.reason)
			before := time.Now()

			require.NoError(t, TimeoutHandler(event))
			after := time.Now()

			requireEphemeralResponse(t, response, test.wantResponse)
			require.Len(t, recorder.calls, 1)
			call := recorder.calls[0]
			require.Equal(t, http.MethodPatch, call.endpoint.Endpoint.Method)
			require.Equal(t, "/guilds/200/members/400", call.endpoint.URL)
			require.Equal(t, 1, call.opts)

			update, ok := call.body.(discord.MemberUpdate)
			require.True(t, ok, "unexpected REST body type %T", call.body)
			require.True(t, update.CommunicationDisabledUntil.OK)
			require.NotNil(t, update.CommunicationDisabledUntil.Value)
			until := *update.CommunicationDisabledUntil.Value
			require.False(t, until.Before(before.Add(test.wantDuration)))
			require.False(t, until.After(after.Add(test.wantDuration)))

			require.NotNil(t, transport.request)
			require.Equal(t, "https://discord.test/guilds/200/members/400", transport.request.URL.String())
			require.Equal(t, test.wantAuditReason, transport.request.Header.Get("X-Audit-Log-Reason"))
			var serialized map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(transport.body, &serialized))
			require.Len(t, serialized, 1)
			require.Contains(t, serialized, "communication_disabled_until")
		})
	}
}

func TestTimeoutHandlerReportsRESTFailure(t *testing.T) {
	recorder, transport := newRecordingRESTClient(t)
	restErr := errors.New("missing permissions")
	recorder.err = restErr
	event, response := newTimeoutEvent(t, recorder, "1h", nil)

	err := TimeoutHandler(event)

	require.ErrorIs(t, err, restErr)
	requireEphemeralResponse(t, response, "Failed to timeout user.")
	require.Len(t, recorder.calls, 1)
	require.Nil(t, transport.request)
}

func TestTimeoutHandlerJoinsRESTAndResponseFailures(t *testing.T) {
	recorder, transport := newRecordingRESTClient(t)
	restErr := errors.New("missing permissions")
	responseErr := errors.New("interaction token expired")
	recorder.err = restErr
	event, response := newTimeoutEvent(t, recorder, "1h", nil)
	response.err = responseErr

	err := TimeoutHandler(event)

	require.ErrorIs(t, err, restErr)
	require.ErrorIs(t, err, responseErr)
	requireEphemeralResponse(t, response, "Failed to timeout user.")
	require.Len(t, recorder.calls, 1)
	require.Nil(t, transport.request)
}

func ptr[T any](value T) *T {
	return &value
}

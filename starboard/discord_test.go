package starboard

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/require"
)

type reactionRequest struct {
	emoji        string
	reactionType discord.MessageReactionType
	after        int
	limit        int
}

type fakeDiscordREST struct {
	reactionPages func(emoji string, reactionType discord.MessageReactionType, after, limit int) ([]discord.User, error)
	reactionCalls []reactionRequest
	getMessage    func(channelID, messageID snowflake.ID) (*discord.Message, error)
	getChannel    func(channelID snowflake.ID) (discord.Channel, error)
	getGuild      func(guildID snowflake.ID) (*discord.RestGuild, error)
	getMember     func(guildID, userID snowflake.ID) (*discord.Member, error)
	getEmoji      func(guildID, emojiID snowflake.ID) (*discord.Emoji, error)
	createMessage func(channelID snowflake.ID, create discord.MessageCreate) (*discord.Message, error)
	updateMessage func(channelID, messageID snowflake.ID, update discord.MessageUpdate) (*discord.Message, error)
	deleteMessage func(channelID, messageID snowflake.ID) error
	addReaction   func(channelID, messageID snowflake.ID, emoji string) error
	getMessages   func(channelID, around, before, after snowflake.ID, limit int) ([]discord.Message, error)
}

func (f *fakeDiscordREST) GetReactions(_ snowflake.ID, _ snowflake.ID, emoji string, reactionType discord.MessageReactionType, after, limit int, _ ...rest.RequestOpt) ([]discord.User, error) {
	f.reactionCalls = append(f.reactionCalls, reactionRequest{emoji: emoji, reactionType: reactionType, after: after, limit: limit})
	return f.reactionPages(emoji, reactionType, after, limit)
}

func (f *fakeDiscordREST) GetMessage(channelID, messageID snowflake.ID, _ ...rest.RequestOpt) (*discord.Message, error) {
	return f.getMessage(channelID, messageID)
}

func (f *fakeDiscordREST) GetChannel(channelID snowflake.ID, _ ...rest.RequestOpt) (discord.Channel, error) {
	return f.getChannel(channelID)
}

func (f *fakeDiscordREST) GetGuild(guildID snowflake.ID, _ bool, _ ...rest.RequestOpt) (*discord.RestGuild, error) {
	return f.getGuild(guildID)
}

func (f *fakeDiscordREST) GetMember(guildID, userID snowflake.ID, _ ...rest.RequestOpt) (*discord.Member, error) {
	return f.getMember(guildID, userID)
}

func (f *fakeDiscordREST) GetEmoji(guildID, emojiID snowflake.ID, _ ...rest.RequestOpt) (*discord.Emoji, error) {
	return f.getEmoji(guildID, emojiID)
}

func (f *fakeDiscordREST) CreateMessage(channelID snowflake.ID, create discord.MessageCreate, _ ...rest.RequestOpt) (*discord.Message, error) {
	return f.createMessage(channelID, create)
}

func (f *fakeDiscordREST) UpdateMessage(channelID, messageID snowflake.ID, update discord.MessageUpdate, _ ...rest.RequestOpt) (*discord.Message, error) {
	return f.updateMessage(channelID, messageID, update)
}

func (f *fakeDiscordREST) DeleteMessage(channelID, messageID snowflake.ID, _ ...rest.RequestOpt) error {
	return f.deleteMessage(channelID, messageID)
}

func (f *fakeDiscordREST) AddReaction(channelID, messageID snowflake.ID, emoji string, _ ...rest.RequestOpt) error {
	return f.addReaction(channelID, messageID, emoji)
}

func (f *fakeDiscordREST) GetMessages(channelID, around, before, after snowflake.ID, limit int, _ ...rest.RequestOpt) ([]discord.Message, error) {
	return f.getMessages(channelID, around, before, after, limit)
}

func TestDiscordTransportVotesPaginatesNormalAndBurstAndFiltersBots(t *testing.T) {
	positiveFirstPage := make([]discord.User, 100)
	for i := range positiveFirstPage {
		positiveFirstPage[i] = discord.User{ID: snowflake.ID(i + 1)}
	}
	positiveFirstPage[9].Bot = true

	fake := &fakeDiscordREST{
		reactionPages: func(emoji string, reactionType discord.MessageReactionType, after, limit int) ([]discord.User, error) {
			require.Equal(t, 100, limit)
			switch {
			case emoji == "star:900" && reactionType == discord.MessageReactionTypeNormal && after == 0:
				return positiveFirstPage, nil
			case emoji == "star:900" && reactionType == discord.MessageReactionTypeNormal && after == 100:
				return []discord.User{{ID: 101}, {ID: 102, Bot: true}}, nil
			case emoji == "star:900" && reactionType == discord.MessageReactionTypeBurst && after == 0:
				return []discord.User{{ID: 1}, {ID: 103}}, nil
			case emoji == "❌" && reactionType == discord.MessageReactionTypeNormal && after == 0:
				return []discord.User{{ID: 40}, {ID: 104}}, nil
			case emoji == "❌" && reactionType == discord.MessageReactionTypeBurst && after == 0:
				return []discord.User{{ID: 104}, {ID: 105, Bot: true}}, nil
			default:
				t.Fatalf("unexpected reaction page: emoji=%q type=%d after=%d", emoji, reactionType, after)
				return nil, nil
			}
		},
	}

	transport := newDiscordTransportWithREST(nil, fake)
	votes, err := transport.Votes(20, 30, "star:900")
	require.NoError(t, err)
	require.Len(t, votes.Positive, 101)
	require.Contains(t, votes.Positive, snowflake.ID(1))
	require.Contains(t, votes.Positive, snowflake.ID(101))
	require.Contains(t, votes.Positive, snowflake.ID(103))
	require.NotContains(t, votes.Positive, snowflake.ID(10))
	require.NotContains(t, votes.Positive, snowflake.ID(102))
	require.ElementsMatch(t, []snowflake.ID{40, 104}, votes.Negative)
	require.Len(t, fake.reactionCalls, 5)
}

func TestDiscordTransportVotesMapsUnknownMessageToNotFound(t *testing.T) {
	fake := &fakeDiscordREST{reactionPages: func(string, discord.MessageReactionType, int, int) ([]discord.User, error) {
		return nil, &rest.Error{Code: rest.JSONErrorCodeUnknownMessage, Message: "Unknown Message"}
	}}
	_, err := newDiscordTransportWithREST(nil, fake).Votes(20, 30, "⭐")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDiscordTransportValidateBoardChecksDestinationPermissionsAndCustomEmojiID(t *testing.T) {
	const (
		guildID   snowflake.ID = 10
		channelID snowflake.ID = 20
		botID     snowflake.ID = 30
		roleID    snowflake.ID = 40
		emojiID   snowflake.ID = 900
	)
	required := discord.PermissionViewChannel |
		discord.PermissionSendMessages |
		discord.PermissionEmbedLinks |
		discord.PermissionReadMessageHistory |
		discord.PermissionAddReactions

	newTransport := func(permissions discord.Permissions, channelGuild snowflake.ID, available bool) *discordTransport {
		fake := &fakeDiscordREST{
			getChannel: func(snowflake.ID) (discord.Channel, error) {
				return guildTextChannel(t, channelID, channelGuild, false, 0), nil
			},
			getGuild: func(id snowflake.ID) (*discord.RestGuild, error) {
				return &discord.RestGuild{Guild: discord.Guild{ID: id}, Roles: []discord.Role{
					{ID: id},
					{ID: roleID, Permissions: permissions},
				}}, nil
			},
			getMember: func(id, userID snowflake.ID) (*discord.Member, error) {
				return &discord.Member{GuildID: id, User: discord.User{ID: userID}, RoleIDs: []snowflake.ID{roleID}}, nil
			},
			getEmoji: func(id, requested snowflake.ID) (*discord.Emoji, error) {
				require.Equal(t, guildID, id)
				require.Equal(t, emojiID, requested)
				return &discord.Emoji{ID: emojiID, GuildID: id, Name: "renamed", Available: available}, nil
			},
		}
		transport := newDiscordTransportWithREST(nil, fake)
		transport.selfID = func() snowflake.ID { return botID }
		return transport
	}

	board := model.Starboard{GuildID: guildID, ChannelID: channelID, Emoji: "old_name:900"}
	require.NoError(t, newTransport(required, guildID, true).ValidateBoard(board))
	require.ErrorContains(t, newTransport(required&^discord.PermissionEmbedLinks, guildID, true).ValidateBoard(board), "permission")
	require.ErrorContains(t, newTransport(required, 99, true).ValidateBoard(board), "guild")
	require.ErrorContains(t, newTransport(required, guildID, false).ValidateBoard(board), "emoji")
}

func TestDiscordTransportPublishUsesNonceSafeQuotePresentationAndSeedsReactions(t *testing.T) {
	var created discord.MessageCreate
	var reactions []string
	fake := &fakeDiscordREST{
		createMessage: func(channelID snowflake.ID, create discord.MessageCreate) (*discord.Message, error) {
			require.Equal(t, snowflake.ID(20), channelID)
			created = create
			return &discord.Message{ID: 70}, nil
		},
		addReaction: func(channelID, messageID snowflake.ID, emoji string) error {
			require.Equal(t, snowflake.ID(20), channelID)
			require.Equal(t, snowflake.ID(70), messageID)
			reactions = append(reactions, emoji)
			return nil
		},
	}
	transport := newDiscordTransportWithREST(nil, fake)
	transport.quoteEmbed = func(*discord.Message) discord.Embed {
		return discord.NewEmbed().WithDescription("quoted source")
	}
	board := model.Starboard{GuildID: 10, ChannelID: 20, Emoji: "⭐"}
	message := &discord.Message{ID: 40, ChannelID: 30}
	votes := Votes{Positive: make([]snowflake.ID, 6), Negative: make([]snowflake.ID, 4)}

	messageID, err := transport.Publish(board, message, votes, "send-once")
	require.NoError(t, err)
	require.Equal(t, snowflake.ID(70), messageID)
	require.Equal(t, "send-once", created.Nonce)
	require.True(t, created.EnforceNonce)
	require.NotNil(t, created.AllowedMentions)
	require.Empty(t, created.AllowedMentions.Parse)
	require.Len(t, created.Embeds, 1)
	require.Equal(t, "quoted source", created.Embeds[0].Description)
	require.Contains(t, created.Embeds[0].URL, "heimdallr_starboard=send-once")
	require.NotContains(t, created.Embeds[0].Description, "send-once")
	require.Len(t, created.Embeds[0].Fields, 1)
	require.Equal(t, "Score", created.Embeds[0].Fields[0].Name)
	require.Contains(t, created.Embeds[0].Fields[0].Value, "6")
	require.Contains(t, created.Embeds[0].Fields[0].Value, "4")
	require.Contains(t, created.Embeds[0].Fields[0].Value, "Net 2")
	require.Len(t, created.Components, 1)
	require.Equal(t, []string{"⭐", "❌"}, reactions)
}

func TestDiscordTransportUpdateRefreshesPresentationWithoutMentions(t *testing.T) {
	var updated discord.MessageUpdate
	fake := &fakeDiscordREST{
		updateMessage: func(channelID, messageID snowflake.ID, update discord.MessageUpdate) (*discord.Message, error) {
			require.Equal(t, snowflake.ID(20), channelID)
			require.Equal(t, snowflake.ID(70), messageID)
			updated = update
			return &discord.Message{ID: messageID}, nil
		},
		addReaction: func(snowflake.ID, snowflake.ID, string) error { return nil },
	}
	transport := newDiscordTransportWithREST(nil, fake)
	transport.quoteEmbed = func(*discord.Message) discord.Embed { return discord.NewEmbed().WithDescription("edited") }
	err := transport.Update(
		model.Starboard{GuildID: 10, ChannelID: 20, Emoji: "⭐"},
		&discord.Message{ID: 40, ChannelID: 30},
		70,
		Votes{Positive: make([]snowflake.ID, 3), Negative: make([]snowflake.ID, 1)},
	)
	require.NoError(t, err)
	require.NotNil(t, updated.AllowedMentions)
	require.NotNil(t, updated.Embeds)
	require.Equal(t, "edited", (*updated.Embeds)[0].Description)
	require.Contains(t, (*updated.Embeds)[0].Fields[0].Value, "Net 2")
}

func TestDiscordTransportPublishMarksDefiniteRejectionAsNotSent(t *testing.T) {
	fake := &fakeDiscordREST{createMessage: func(snowflake.ID, discord.MessageCreate) (*discord.Message, error) {
		return nil, &rest.Error{Response: &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden"}}
	}}
	transport := newDiscordTransportWithREST(nil, fake)
	transport.quoteEmbed = func(*discord.Message) discord.Embed { return discord.Embed{} }
	_, err := transport.Publish(model.Starboard{GuildID: 10, ChannelID: 20, Emoji: "⭐"}, &discord.Message{ID: 40, ChannelID: 30}, Votes{}, "nonce")
	require.ErrorIs(t, err, ErrNotSent)
}

func TestDiscordTransportPublishMarksPreflightFailureAsNotSent(t *testing.T) {
	fake := &fakeDiscordREST{
		getMember: func(snowflake.ID, snowflake.ID) (*discord.Member, error) {
			return &discord.Member{User: discord.User{ID: 30}}, nil
		},
		getEmoji: func(snowflake.ID, snowflake.ID) (*discord.Emoji, error) {
			return nil, errors.New("emoji unavailable")
		},
		createMessage: func(snowflake.ID, discord.MessageCreate) (*discord.Message, error) {
			t.Fatal("preflight failure must not create a message")
			return nil, nil
		},
	}
	transport := newDiscordTransportWithREST(nil, fake)
	transport.selfID = func() snowflake.ID { return 30 }
	transport.quoteEmbed = func(*discord.Message) discord.Embed { return discord.Embed{} }
	_, err := transport.Publish(model.Starboard{GuildID: 10, ChannelID: 20, Emoji: "old:900"}, &discord.Message{ID: 40, ChannelID: 30}, Votes{}, "nonce")
	require.ErrorIs(t, err, ErrNotSent)
}

func TestDiscordTransportDeleteIsIdempotentOnlyForUnknownMessage(t *testing.T) {
	fake := &fakeDiscordREST{deleteMessage: func(snowflake.ID, snowflake.ID) error {
		return &rest.Error{Code: rest.JSONErrorCodeUnknownMessage, Message: "Unknown Message"}
	}}
	require.NoError(t, newDiscordTransportWithREST(nil, fake).Delete(20, 30))

	forbidden := errors.New("forbidden")
	fake.deleteMessage = func(snowflake.ID, snowflake.ID) error { return forbidden }
	require.ErrorIs(t, newDiscordTransportWithREST(nil, fake).Delete(20, 30), forbidden)
}

func TestDiscordTransportRecoverSearchesHistoryByNonceWithoutSending(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	start := snowflake.New(since)
	calls := 0
	fake := &fakeDiscordREST{
		getMessages: func(channelID, around, before, after snowflake.ID, limit int) ([]discord.Message, error) {
			require.Equal(t, snowflake.ID(20), channelID)
			require.Zero(t, around)
			require.Zero(t, after)
			require.Equal(t, 100, limit)
			calls++
			if calls == 1 {
				require.Zero(t, before)
				page := make([]discord.Message, 100)
				for i := range page {
					page[i] = discord.Message{ID: start + snowflake.ID(200-i), Nonce: discord.Nonce("other"), Author: discord.User{ID: 30}}
				}
				return page, nil
			}
			require.Equal(t, start+101, before)
			return []discord.Message{{
				ID:     start + 100,
				Author: discord.User{ID: 30},
				Embeds: []discord.Embed{{URL: "https://discord.com/channels/10/30/40?heimdallr_starboard=recover-me"}},
			}}, nil
		},
		createMessage: func(snowflake.ID, discord.MessageCreate) (*discord.Message, error) {
			t.Fatal("Recover must never create a message")
			return nil, nil
		},
	}
	transport := newDiscordTransportWithREST(nil, fake)
	transport.selfID = func() snowflake.ID { return 30 }

	messageID, err := transport.Recover(model.Starboard{ChannelID: 20}, "recover-me", since)
	require.NoError(t, err)
	require.Equal(t, start+100, messageID)
	require.Equal(t, 2, calls)
}

func TestDiscordTransportRecoverDistinguishesRecentAmbiguityFromAgedExhaustiveMiss(t *testing.T) {
	fake := recoveryPermissionFake(t, discord.PermissionViewChannel|discord.PermissionReadMessageHistory)
	transport := newDiscordTransportWithREST(nil, fake)
	transport.selfID = func() snowflake.ID { return 30 }

	messageID, err := transport.Recover(model.Starboard{GuildID: 10, ChannelID: 20}, "missing", time.Now().Add(-time.Minute))
	require.NoError(t, err)
	require.Zero(t, messageID)

	messageID, err = transport.Recover(model.Starboard{GuildID: 10, ChannelID: 20}, "missing", time.Now().Add(-15*time.Minute))
	require.ErrorIs(t, err, ErrNotSent)
	require.Zero(t, messageID)
}

func TestDiscordTransportRecoverCannotConcludeNotSentWithoutHistoryPermission(t *testing.T) {
	fake := recoveryPermissionFake(t, discord.PermissionViewChannel)
	transport := newDiscordTransportWithREST(nil, fake)
	transport.selfID = func() snowflake.ID { return 30 }
	messageID, err := transport.Recover(model.Starboard{GuildID: 10, ChannelID: 20}, "missing", time.Now().Add(-15*time.Minute))
	require.NoError(t, err)
	require.Zero(t, messageID)
}

func TestDiscordTransportRecoverKeepsAgedMissAmbiguousWhenSearchBoundIsReached(t *testing.T) {
	nextID := snowflake.New(time.Now())
	fake := &fakeDiscordREST{getMessages: func(_ snowflake.ID, _, before, _ snowflake.ID, _ int) ([]discord.Message, error) {
		if before != 0 {
			nextID = before - 100
		}
		page := make([]discord.Message, 100)
		for i := range page {
			page[i].ID = nextID - snowflake.ID(i)
		}
		return page, nil
	}}
	transport := newDiscordTransportWithREST(nil, fake)
	transport.selfID = func() snowflake.ID { return 30 }
	messageID, err := transport.Recover(model.Starboard{ChannelID: 20}, "missing", time.Now().Add(-15*time.Minute))
	require.NoError(t, err)
	require.Zero(t, messageID)
}

func TestDiscordTransportEligibleEnforcesPublicSourceAndAgeRestriction(t *testing.T) {
	const (
		guildID     snowflake.ID = 10
		sourceID    snowflake.ID = 20
		destination snowflake.ID = 30
		threadID    snowflake.ID = 40
	)
	publicPermissions := discord.PermissionViewChannel | discord.PermissionReadMessageHistory
	board := model.Starboard{GuildID: guildID, ChannelID: destination}
	message := &discord.Message{ID: 50, ChannelID: sourceID, Author: discord.User{ID: 60}}

	tests := []struct {
		name          string
		message       *discord.Message
		source        discord.Channel
		parentChannel discord.Channel
		destNSFW      bool
		want          bool
		parent        snowflake.ID
	}{
		{name: "public text source", message: message, source: guildTextChannel(t, sourceID, guildID, false, 0), want: true, parent: sourceID},
		{name: "hidden from everyone", message: message, source: guildTextChannel(t, sourceID, guildID, false, discord.PermissionViewChannel), want: false, parent: sourceID},
		{name: "public thread inherits parent", message: &discord.Message{ID: 50, ChannelID: threadID, Author: discord.User{ID: 60}}, source: guildThread(t, threadID, sourceID, guildID, discord.ChannelTypeGuildPublicThread), want: true, parent: sourceID},
		{name: "public forum thread", message: &discord.Message{ID: 50, ChannelID: threadID, Author: discord.User{ID: 60}}, source: guildThread(t, threadID, sourceID, guildID, discord.ChannelTypeGuildPublicThread), parentChannel: guildForumChannel(t, sourceID, guildID, false), want: true, parent: sourceID},
		{name: "nsfw forum thread into unrestricted destination", message: &discord.Message{ID: 50, ChannelID: threadID, Author: discord.User{ID: 60}}, source: guildThread(t, threadID, sourceID, guildID, discord.ChannelTypeGuildPublicThread), parentChannel: guildForumChannel(t, sourceID, guildID, true), want: false, parent: sourceID},
		{name: "private thread", message: &discord.Message{ID: 50, ChannelID: threadID, Author: discord.User{ID: 60}}, source: guildThread(t, threadID, sourceID, guildID, discord.ChannelTypeGuildPrivateThread), want: false, parent: sourceID},
		{name: "nsfw into unrestricted destination", message: message, source: guildTextChannel(t, sourceID, guildID, true, 0), want: false, parent: sourceID},
		{name: "nsfw into nsfw destination", message: message, source: guildTextChannel(t, sourceID, guildID, true, 0), destNSFW: true, want: true, parent: sourceID},
		{name: "bot message", message: &discord.Message{ID: 50, ChannelID: sourceID, Author: discord.User{ID: 60, Bot: true}}, source: guildTextChannel(t, sourceID, guildID, false, 0), want: false, parent: sourceID},
		{name: "webhook message", message: func() *discord.Message {
			id := snowflake.ID(70)
			return &discord.Message{ID: 50, ChannelID: sourceID, Author: discord.User{ID: 60}, WebhookID: &id}
		}(), source: guildTextChannel(t, sourceID, guildID, false, 0), want: false, parent: sourceID},
		{name: "board destination as source", message: &discord.Message{ID: 50, ChannelID: destination, Author: discord.User{ID: 60}}, source: guildTextChannel(t, destination, guildID, false, 0), want: false, parent: destination},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeDiscordREST{
				getChannel: func(id snowflake.ID) (discord.Channel, error) {
					switch id {
					case tt.message.ChannelID:
						return tt.source, nil
					case sourceID:
						if tt.parentChannel != nil {
							return tt.parentChannel, nil
						}
						return guildTextChannel(t, sourceID, guildID, tt.source.(discord.GuildMessageChannel).NSFW(), 0), nil
					case destination:
						return guildTextChannel(t, destination, guildID, tt.destNSFW, 0), nil
					default:
						return nil, fmt.Errorf("unexpected channel %d", id)
					}
				},
				getGuild: func(id snowflake.ID) (*discord.RestGuild, error) {
					return &discord.RestGuild{Guild: discord.Guild{ID: id}, Roles: []discord.Role{{ID: id, Permissions: publicPermissions}}}, nil
				},
			}
			eligible, parent, err := newDiscordTransportWithREST(nil, fake).Eligible(board, tt.message)
			require.NoError(t, err)
			require.Equal(t, tt.want, eligible)
			require.Equal(t, tt.parent, parent)
		})
	}
}

func recoveryPermissionFake(t *testing.T, permissions discord.Permissions) *fakeDiscordREST {
	t.Helper()
	return &fakeDiscordREST{
		getMessages: func(snowflake.ID, snowflake.ID, snowflake.ID, snowflake.ID, int) ([]discord.Message, error) {
			return nil, nil
		},
		getChannel: func(snowflake.ID) (discord.Channel, error) {
			return guildTextChannel(t, 20, 10, false, 0), nil
		},
		getGuild: func(snowflake.ID) (*discord.RestGuild, error) {
			return &discord.RestGuild{Guild: discord.Guild{ID: 10}, Roles: []discord.Role{{ID: 10, Permissions: permissions}}}, nil
		},
		getMember: func(snowflake.ID, snowflake.ID) (*discord.Member, error) {
			return &discord.Member{GuildID: 10, User: discord.User{ID: 30}}, nil
		},
	}
}

func TestDiscordTransportSourceConfirmsGuildAndMapsOnlyUnknownMessage(t *testing.T) {
	t.Run("source belongs to requested guild", func(t *testing.T) {
		fake := &fakeDiscordREST{
			getMessage: func(channelID, messageID snowflake.ID) (*discord.Message, error) {
				return &discord.Message{ID: messageID, ChannelID: channelID}, nil
			},
			getChannel: func(snowflake.ID) (discord.Channel, error) {
				return guildTextChannel(t, 20, 10, false, 0), nil
			},
		}
		message, err := newDiscordTransportWithREST(nil, fake).Source(10, 20, 30)
		require.NoError(t, err)
		require.Equal(t, snowflake.ID(30), message.ID)
	})

	t.Run("cross-guild channel is rejected", func(t *testing.T) {
		messageFetched := false
		fake := &fakeDiscordREST{
			getMessage: func(channelID, messageID snowflake.ID) (*discord.Message, error) {
				messageFetched = true
				return &discord.Message{ID: messageID, ChannelID: channelID}, nil
			},
			getChannel: func(snowflake.ID) (discord.Channel, error) {
				return guildTextChannel(t, 20, 99, false, 0), nil
			},
		}
		_, err := newDiscordTransportWithREST(nil, fake).Source(10, 20, 30)
		require.Error(t, err)
		require.False(t, messageFetched)
	})

	t.Run("unknown message is definitive not found", func(t *testing.T) {
		fake := &fakeDiscordREST{
			getChannel: func(snowflake.ID) (discord.Channel, error) {
				return guildTextChannel(t, 20, 10, false, 0), nil
			},
			getMessage: func(snowflake.ID, snowflake.ID) (*discord.Message, error) {
				return nil, &rest.Error{Code: rest.JSONErrorCodeUnknownMessage, Message: "Unknown Message"}
			},
		}
		_, err := newDiscordTransportWithREST(nil, fake).Source(10, 20, 30)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("other REST failures remain distinguishable", func(t *testing.T) {
		forbidden := errors.New("forbidden")
		fake := &fakeDiscordREST{
			getChannel: func(snowflake.ID) (discord.Channel, error) {
				return guildTextChannel(t, 20, 10, false, 0), nil
			},
			getMessage: func(snowflake.ID, snowflake.ID) (*discord.Message, error) {
				return nil, forbidden
			},
		}
		_, err := newDiscordTransportWithREST(nil, fake).Source(10, 20, 30)
		require.ErrorIs(t, err, forbidden)
		require.NotErrorIs(t, err, ErrNotFound)
	})
}

func guildTextChannel(t *testing.T, channelID, guildID snowflake.ID, nsfw bool, denyEveryone discord.Permissions) discord.GuildTextChannel {
	t.Helper()
	overwrites := "[]"
	if denyEveryone != 0 {
		overwrites = fmt.Sprintf(`[{"id":"%s","type":0,"allow":"0","deny":"%d"}]`, guildID, denyEveryone)
	}
	channelJSON := []byte(fmt.Sprintf(`{"id":"%s","guild_id":"%s","type":0,"name":"channel","nsfw":%t,"permission_overwrites":%s}`, channelID, guildID, nsfw, overwrites))
	var channel discord.GuildTextChannel
	require.NoError(t, channel.UnmarshalJSON(channelJSON))
	return channel
}

func guildThread(t *testing.T, channelID, parentID, guildID snowflake.ID, channelType discord.ChannelType) discord.GuildThread {
	t.Helper()
	channelJSON := []byte(fmt.Sprintf(`{"id":"%s","guild_id":"%s","parent_id":"%s","type":%d,"name":"thread","thread_metadata":{"archived":false,"auto_archive_duration":60,"archive_timestamp":"2026-09-16T00:00:00Z","locked":false}}`, channelID, guildID, parentID, channelType))
	var channel discord.GuildThread
	require.NoError(t, channel.UnmarshalJSON(channelJSON))
	return channel
}

func guildForumChannel(t *testing.T, channelID, guildID snowflake.ID, nsfw bool) discord.GuildForumChannel {
	t.Helper()
	channelJSON := []byte(fmt.Sprintf(`{"id":"%s","guild_id":"%s","type":15,"name":"forum","nsfw":%t,"permission_overwrites":[]}`, channelID, guildID, nsfw))
	var channel discord.GuildForumChannel
	require.NoError(t, channel.UnmarshalJSON(channelJSON))
	return channel
}

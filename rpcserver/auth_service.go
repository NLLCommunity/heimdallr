package rpcserver

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/disgoorg/disgo/bot"

	heimdallrv1 "github.com/NLLCommunity/heimdallr/gen/heimdallr/v1"
	"github.com/NLLCommunity/heimdallr/model"
)

type authService struct {
	client *bot.Client
}

func (s *authService) GetLoginURL(_ context.Context, _ *heimdallrv1.GetLoginURLRequest) (*heimdallrv1.GetLoginURLResponse, error) {
	return nil, connect.NewError(
		connect.CodeUnimplemented,
		errors.New("use the /admin-dashboard command in Discord to get a login link"),
	)
}

func (s *authService) ExchangeCode(_ context.Context, req *heimdallrv1.ExchangeCodeRequest) (*heimdallrv1.ExchangeCodeResponse, error) {
	session, err := model.ExchangeLoginCode(req.GetCode())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or expired login code"))
	}

	return &heimdallrv1.ExchangeCodeResponse{
		Token: session.Token,
		User: &heimdallrv1.User{
			Id:       session.UserID.String(),
			Username: session.Username,
			Avatar:   session.Avatar,
		},
	}, nil
}

func (s *authService) GetCurrentUser(ctx context.Context, _ *heimdallrv1.GetCurrentUserRequest) (*heimdallrv1.GetCurrentUserResponse, error) {
	session := SessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	return &heimdallrv1.GetCurrentUserResponse{
		User: &heimdallrv1.User{
			Id:       session.UserID.String(),
			Username: session.Username,
			Avatar:   session.Avatar,
		},
	}, nil
}

func (s *authService) ListGuilds(ctx context.Context, _ *heimdallrv1.ListGuildsRequest) (*heimdallrv1.ListGuildsResponse, error) {
	session := SessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	var guilds []*heimdallrv1.Guild

	for guild := range s.client.Caches.Guilds() {
		if !isGuildAdmin(s.client, guild, session.UserID) {
			continue
		}

		var icon string
		if guild.Icon != nil {
			icon = *guild.Icon
		}
		guilds = append(guilds, &heimdallrv1.Guild{
			Id:   guild.ID.String(),
			Name: guild.Name,
			Icon: icon,
		})
	}

	slog.Debug("ListGuilds", "user_id", session.UserID, "count", len(guilds))

	return &heimdallrv1.ListGuildsResponse{
		Guilds: guilds,
	}, nil
}

func (s *authService) Logout(ctx context.Context, _ *heimdallrv1.LogoutRequest) (*heimdallrv1.LogoutResponse, error) {
	session := SessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if err := model.DeleteSession(session.Token); err != nil {
		slog.Error("failed to delete session", "error", err)
	}

	return &heimdallrv1.LogoutResponse{}, nil
}

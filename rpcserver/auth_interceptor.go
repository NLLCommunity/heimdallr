package rpcserver

import (
	"context"
	"strings"

	"connectrpc.com/connect"

	"github.com/NLLCommunity/heimdallr/gen/heimdallr/v1/heimdallrv1connect"
	"github.com/NLLCommunity/heimdallr/model"
)

type sessionContextKey struct{}

func SessionFromContext(ctx context.Context) *model.DashboardSession {
	session, _ := ctx.Value(sessionContextKey{}).(*model.DashboardSession)
	return session
}

func newAuthInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Skip auth for ExchangeCode — no token yet.
			if req.Spec().Procedure == heimdallrv1connect.AuthServiceExchangeCodeProcedure ||
				req.Spec().Procedure == heimdallrv1connect.AuthServiceGetLoginURLProcedure {
				return next(ctx, req)
			}

			token := req.Header().Get("Authorization")
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			token = strings.TrimPrefix(token, "Bearer ")
			session, err := model.GetSession(token)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			ctx = context.WithValue(ctx, sessionContextKey{}, session)
			return next(ctx, req)
		}
	}
}

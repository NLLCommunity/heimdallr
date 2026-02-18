package rpcserver

import (
	"context"
	"net/http"
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

// cookieJarKey is the context key for the mutable cookie jar.
type cookieJarKey struct{}

// cookieJar holds cookies to be set on the HTTP response.
type cookieJar struct {
	cookies []*http.Cookie
}

// SetResponseCookie schedules a Set-Cookie header on the response.
// Must be called from a handler running inside newCookieInterceptor.
func SetResponseCookie(ctx context.Context, cookie *http.Cookie) {
	jar, ok := ctx.Value(cookieJarKey{}).(*cookieJar)
	if ok {
		jar.cookies = append(jar.cookies, cookie)
	}
}

// newCookieInterceptor injects a cookie jar into the context and writes
// any accumulated Set-Cookie headers onto the Connect response.
func newCookieInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			jar := &cookieJar{}
			ctx = context.WithValue(ctx, cookieJarKey{}, jar)
			resp, err := next(ctx, req)
			if resp != nil {
				for _, c := range jar.cookies {
					resp.Header().Add("Set-Cookie", c.String())
				}
			}
			return resp, err
		}
	}
}

func newAuthInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Skip auth for ExchangeCode and GetLoginURL — no token yet.
			if req.Spec().Procedure == heimdallrv1connect.AuthServiceExchangeCodeProcedure ||
				req.Spec().Procedure == heimdallrv1connect.AuthServiceGetLoginURLProcedure {
				return next(ctx, req)
			}

			// Try cookie first, fall back to Authorization header.
			token := tokenFromCookie(req.Header())
			if token == "" {
				token = strings.TrimPrefix(req.Header().Get("Authorization"), "Bearer ")
			}
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			session, err := model.GetSession(token)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			ctx = context.WithValue(ctx, sessionContextKey{}, session)
			return next(ctx, req)
		}
	}
}

const sessionCookieName = "heimdallr_session"

func tokenFromCookie(h http.Header) string {
	raw := h.Get("Cookie")
	if raw == "" {
		return ""
	}
	// Parse cookies from the header.
	header := http.Header{"Cookie": {raw}}
	request := http.Request{Header: header}
	c, err := request.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

package rpcserver

import (
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/disgoorg/disgo/bot"

	"github.com/NLLCommunity/heimdallr/gen/heimdallr/v1/heimdallrv1connect"
	"github.com/NLLCommunity/heimdallr/model"
)

func StartServer(addr string, discordClient *bot.Client) error {
	mux := http.NewServeMux()

	interceptors := connect.WithInterceptors(
		validate.NewInterceptor(),
		newAuthInterceptor(),
	)

	authSvc := &authService{client: discordClient}
	authPath, authHandler := heimdallrv1connect.NewAuthServiceHandler(authSvc, interceptors)
	mux.Handle(authPath, authHandler)

	settingsSvc := &guildSettingsService{client: discordClient}
	settingsPath, settingsHandler := heimdallrv1connect.NewGuildSettingsServiceHandler(settingsSvc, interceptors)
	mux.Handle(settingsPath, settingsHandler)

	reflector := grpcreflect.NewStaticReflector(
		heimdallrv1connect.AuthServiceName,
		heimdallrv1connect.GuildSettingsServiceName,
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// Clean expired sessions periodically.
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			model.CleanExpiredSessions()
		}
	}()

	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)

	s := http.Server{
		Addr:      addr,
		Handler:   mux,
		Protocols: p,
	}

	slog.Info("Starting RPC server on address: " + addr)
	return s.ListenAndServe()
}

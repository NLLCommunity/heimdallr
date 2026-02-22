//go:build web

package rpcserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/disgoorg/disgo/bot"
	"github.com/rs/cors"
	"github.com/spf13/viper"

	"github.com/NLLCommunity/heimdallr/gen/heimdallr/v1/heimdallrv1connect"
	"github.com/NLLCommunity/heimdallr/model"
)

func StartServer(addr string, discordClient *bot.Client) error {
	baseURL := viper.GetString("dashboard.base_url")
	devMode := viper.GetBool("dev_mode.enabled")

	// Validate dashboard.base_url.
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("invalid dashboard.base_url %q: %w", baseURL, err)
	}
	if !devMode && parsedURL.Scheme == "http" {
		slog.Warn("dashboard.base_url uses http — this is insecure in production", "url", baseURL)
	}

	// Warn about missing TLS in production.
	if !devMode {
		slog.Warn("RPC server does not terminate TLS — use a reverse proxy (e.g. nginx, Caddy) with TLS in production")
	}

	mux := http.NewServeMux()

	interceptors := connect.WithInterceptors(
		newCookieInterceptor(),
		validate.NewInterceptor(),
		newAuthInterceptor(),
	)

	authSvc := &authService{client: discordClient}
	authPath, authHandler := heimdallrv1connect.NewAuthServiceHandler(authSvc, interceptors)
	mux.Handle(authPath, authHandler)

	settingsSvc := &guildSettingsService{client: discordClient}
	settingsPath, settingsHandler := heimdallrv1connect.NewGuildSettingsServiceHandler(settingsSvc, interceptors)
	mux.Handle(settingsPath, settingsHandler)

	// Only expose gRPC reflection in dev mode.
	if devMode {
		reflector := grpcreflect.NewStaticReflector(
			heimdallrv1connect.AuthServiceName,
			heimdallrv1connect.GuildSettingsServiceName,
		)
		mux.Handle(grpcreflect.NewHandlerV1(reflector))
		mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}

	// Serve embedded frontend as SPA fallback (catch-all for non-RPC paths).
	mux.Handle("/", newSPAHandler())

	// Clean expired sessions periodically.
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			model.CleanExpiredSessions()
		}
	}()

	// CORS configuration — only allow the dashboard origin.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{baseURL},
		AllowedMethods:   []string{"POST", "GET", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Connect-Protocol-Version"},
		AllowCredentials: true,
	}).Handler(mux)

	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)

	s := http.Server{
		Addr:              addr,
		Handler:           corsHandler,
		Protocols:         p,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("Starting RPC server on address: " + addr)
	return s.ListenAndServe()
}

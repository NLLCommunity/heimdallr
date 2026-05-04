package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"

	"github.com/NLLCommunity/heimdallr/model"
)

func StartServer(ctx context.Context, addr string, client *bot.Client) error {
	baseURL := viper.GetString("dashboard.base_url")
	devMode := viper.GetBool("dev_mode.enabled")

	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("invalid dashboard.base_url %q: %w", baseURL, err)
	}
	if !devMode && parsedURL.Scheme == "http" {
		slog.Warn("dashboard.base_url uses http — this is insecure in production", "url", baseURL)
	}
	allowedOrigin := parsedURL.Scheme + "://" + parsedURL.Host

	trustedProxies, err := parseTrustedProxies(viper.GetStringSlice("web.trusted_proxies"))
	if err != nil {
		return fmt.Errorf("web.trusted_proxies: %w", err)
	}
	if len(trustedProxies) == 0 {
		slog.Info("web.trusted_proxies is empty — forwarded-IP headers (X-Real-IP / X-Forwarded-For) will be ignored")
	}

	mux := http.NewServeMux()

	// Auth routes.
	mux.HandleFunc("GET /", handleRoot)
	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("GET /callback", handleCallbackGET)
	mux.HandleFunc("POST /callback", handleCallbackPOST(allowedOrigin))
	mux.HandleFunc("GET /logout", handleLogout)

	// Guild routes.
	mux.HandleFunc("GET /guilds", handleGuilds(client))
	mux.HandleFunc("GET /guild/{id}", handleDashboard(client))

	// Settings POST routes.
	mux.HandleFunc("POST /guild/{id}/settings/mod-channel", handleSaveModChannel(client))
	mux.HandleFunc("POST /guild/{id}/settings/infractions", handleSaveInfractions(client))
	mux.HandleFunc("POST /guild/{id}/settings/anti-spam", handleSaveAntiSpam(client))
	mux.HandleFunc("POST /guild/{id}/settings/ban-footer", handleSaveBanFooter(client))
	mux.HandleFunc("POST /guild/{id}/settings/modmail", handleSaveModmail(client))
	mux.HandleFunc("POST /guild/{id}/settings/gatekeep", handleSaveGatekeep(client))
	mux.HandleFunc("POST /guild/{id}/settings/join-leave", handleSaveJoinLeave(client))

	// Per-session rate limiter for sandbox sends — keyed by user ID rather
	// than IP, since the threat is admin abuse, not anonymous flooding.
	sandboxLimiter := newIPRateLimiter(
		rate.Every(time.Minute/sandboxRatePerMinute),
		sandboxBurst,
	)

	mux.HandleFunc("GET /guild/{id}/sandbox", handleSandbox(client))
	mux.HandleFunc("POST /guild/{id}/sandbox/send", handleSandboxSend(client, sandboxLimiter))

	// Static files.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(getStaticFS())))

	exchangeCodeLimiter := newIPRateLimiter(
		rate.Every(time.Minute/exchangeCodeRatePerMinute),
		exchangeCodeBurst,
	)

	// Middleware chain: mux → auth → body limit → rate limit → CORS.
	withAuth := authMiddleware(mux)
	withBodyLimit := bodyLimitMiddleware(withAuth)
	withRateLimit := rateLimitMiddleware(
		exchangeCodeLimiter, trustedProxies,
		rateLimitRule{Method: http.MethodPost, Path: "/callback"},
	)(withBodyLimit)
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{allowedOrigin},
		AllowedMethods: []string{"POST", "GET", "OPTIONS"},
		AllowedHeaders: []string{
			"Content-Type",
			// Full set of HTMX 2.x request headers — keep in sync with
			// https://htmx.org/reference/#request_headers so cross-origin
			// preflights don't fail when HTMX sends them.
			"HX-Boosted",
			"HX-Current-URL",
			"HX-History-Restore-Request",
			"HX-Prompt",
			"HX-Request",
			"HX-Target",
			"HX-Trigger",
			"HX-Trigger-Name",
		},
		// Single-origin dashboard, so CORS isn't strictly needed. Keeping the
		// middleware as defense-in-depth and AllowCredentials=true so any
		// future cross-origin embed (e.g. an admin tool on a sibling
		// subdomain) can opt in by matching AllowedOrigins.
		AllowCredentials: true,
	}).Handler(withRateLimit)

	// Session cleanup ticker — exits when ctx is cancelled.
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := model.CleanExpiredSessions(); err != nil {
					slog.Warn("session cleanup failed", "error", err)
				}
				exchangeCodeLimiter.cleanup(rateLimiterTTL)
				sandboxLimiter.cleanup(rateLimiterTTL)
			}
		}
	}()

	s := &http.Server{
		Addr:              addr,
		Handler:           corsHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Starting web server", "addr", addr)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		slog.Info("Shutting down web server")
		if err := s.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("web server shutdown: %w", err)
		}
		return nil
	case err := <-serverErr:
		return err
	}
}

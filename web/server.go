package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"

	"github.com/NLLCommunity/heimdallr/config"
	"github.com/NLLCommunity/heimdallr/model"
)

func StartServer(ctx context.Context, addr string, client *bot.Client) error {
	devMode := viper.GetBool("dev_mode.enabled")

	parsedURL, err := config.ParsedDashboardBaseURL()
	if err != nil {
		return err
	}
	if !devMode && parsedURL.Scheme == "http" {
		slog.Warn("dashboard.base_url uses http — this is insecure in production", "url", parsedURL.String())
	}
	allowedOrigin := canonicalOrigin(parsedURL)
	secureCookie := parsedURL.Scheme == "https"

	// Fail fast on missing OAuth config: every admin path now requires the
	// OAuth handshake to populate user-scoped tokens. A bot started
	// without these would render a login page that leads nowhere.
	oauthClientID, err := config.DiscordClientID()
	if err != nil {
		return err
	}
	oauthClientSecret, err := config.DiscordClientSecret()
	if err != nil {
		return err
	}
	tokenCrypto, err := model.NewTokenCrypto()
	if err != nil {
		return err
	}
	oauthRedirect := oauthRedirectURI(parsedURL)

	trustedProxies, err := parseTrustedProxies(viper.GetStringSlice("web.trusted_proxies"))
	if err != nil {
		return fmt.Errorf("web.trusted_proxies: %w", err)
	}
	if len(trustedProxies) == 0 {
		slog.Info("web.trusted_proxies is empty — forwarded-IP headers (X-Real-IP / X-Forwarded-For) will be ignored")
	}

	mux := http.NewServeMux()

	// Auth routes. /oauth/start is public (it's the entry point for
	// first-time logins from /login as well as the post-login redirect
	// target for deep-links).
	mux.HandleFunc("GET /", handleRoot)
	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("GET /oauth/start", handleOAuthStart(oauthClientID, oauthRedirect, secureCookie))
	mux.HandleFunc("GET /oauth/callback", handleOAuthCallback(client, oauthClientID, oauthClientSecret, oauthRedirect, tokenCrypto, secureCookie))
	mux.HandleFunc("GET /logout", handleLogout(secureCookie))

	// Guild routes.
	mux.HandleFunc("GET /guilds", handleGuilds(client, oauthClientID, oauthClientSecret, tokenCrypto))
	mux.HandleFunc("GET /guild/{id}", handleDashboard(client))

	// Settings POST routes.
	mux.HandleFunc("POST /guild/{id}/settings/mod-channel", handleSaveModChannel(client))
	mux.HandleFunc("POST /guild/{id}/settings/infractions", handleSaveInfractions(client))
	mux.HandleFunc("POST /guild/{id}/settings/anti-spam", handleSaveAntiSpam(client))
	mux.HandleFunc("POST /guild/{id}/settings/ban-footer", handleSaveBanFooter(client))
	mux.HandleFunc("POST /guild/{id}/settings/modmail", handleSaveModmail(client))
	mux.HandleFunc("POST /guild/{id}/settings/gatekeep", handleSaveGatekeep(client))
	mux.HandleFunc("POST /guild/{id}/settings/join-leave", handleSaveJoinLeave(client))
	mux.HandleFunc("POST /guild/{id}/settings/posts", handleSavePosts(client))

	mux.HandleFunc("GET /guild/{id}/auditlog", handleAuditLog(client))
	mux.HandleFunc("POST /guild/{id}/settings/audit-log", handleSaveAuditLog(client))

	// Per-session rate limiter for sandbox sends — keyed by user ID rather
	// than IP, since the threat is admin abuse, not anonymous flooding.
	sandboxLimiter := newKeyedRateLimiter(
		rate.Every(time.Minute/sandboxRatePerMinute),
		sandboxBurst,
	)

	mux.HandleFunc("GET /guild/{id}/sandbox", handleSandbox(client))
	mux.HandleFunc("POST /guild/{id}/sandbox/send", handleSandboxSend(client, sandboxLimiter))
	mux.HandleFunc("POST /guild/{id}/sandbox/load", handleSandboxLoad(client, sandboxLimiter))
	mux.HandleFunc("POST /guild/{id}/sandbox/edit", handleSandboxEdit(client, sandboxLimiter))

	// Per-user rate limiter for post publish/unpublish/delete. Save/list/
	// editor/preview don't hit Discord, so they're left unlimited beyond the
	// global body-limit + auth gate.
	postsLimiter := newKeyedRateLimiter(
		rate.Every(time.Minute/postsDiscordRatePerMinute),
		postsDiscordBurst,
	)

	mux.HandleFunc("GET /guild/{id}/posts", handlePostsList(client))
	mux.HandleFunc("GET /guild/{id}/posts/new", handlePostsNew(client))
	mux.HandleFunc("POST /guild/{id}/posts", handlePostsCreate(client))
	mux.HandleFunc("POST /guild/{id}/posts/preview", handlePostPreview(client))
	mux.HandleFunc("GET /guild/{id}/posts/{postID}", handlePostEditor(client))
	mux.HandleFunc("POST /guild/{id}/posts/{postID}", handlePostSave(client))
	mux.HandleFunc("POST /guild/{id}/posts/{postID}/preview", handlePostPreview(client))
	mux.HandleFunc("POST /guild/{id}/posts/{postID}/publish", handlePostPublish(client, postsLimiter))
	mux.HandleFunc("POST /guild/{id}/posts/{postID}/unpublish", handlePostUnpublish(client, postsLimiter))
	mux.HandleFunc("POST /guild/{id}/posts/{postID}/delete", handlePostDelete(client, postsLimiter))

	// Static files.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(getStaticFS())))

	// /oauth/start creates a state row + cookie per call, so an
	// unauthenticated attacker spraying it would otherwise grow the
	// state table unboundedly until the 15-minute janitor catches up.
	// /oauth/callback is just as unauthenticated and opens a
	// write-capable ConsumeOAuthState transaction per request - the
	// state-cookie equality check is attacker-satisfiable since the
	// client controls both cookie and query param - so it shares the
	// same per-IP budget.
	oauthLimiter := newKeyedRateLimiter(
		rate.Every(time.Minute/oauthRatePerMinute),
		oauthBurst,
	)

	// Middleware chain: mux → auth → body limit → rate limit → CORS.
	withAuth := authMiddleware(mux)
	withBodyLimit := bodyLimitMiddleware(withAuth)
	withRateLimit := rateLimitMiddleware(
		oauthLimiter, trustedProxies,
		rateLimitRule{Method: http.MethodGet, Path: "/oauth/start"},
		rateLimitRule{Method: http.MethodGet, Path: "/oauth/callback"},
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
				oauthLimiter.cleanup(rateLimiterTTL)
				sandboxLimiter.cleanup(rateLimiterTTL)
				postsLimiter.cleanup(rateLimiterTTL)
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
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

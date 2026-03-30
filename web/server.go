package web

import (
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

func StartServer(addr string, client *bot.Client) error {
	baseURL := viper.GetString("dashboard.base_url")
	devMode := viper.GetBool("dev_mode.enabled")

	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("invalid dashboard.base_url %q: %w", baseURL, err)
	}
	if !devMode && parsedURL.Scheme == "http" {
		slog.Warn("dashboard.base_url uses http — this is insecure in production", "url", baseURL)
	}

	mux := http.NewServeMux()

	// Auth routes.
	mux.HandleFunc("GET /", handleRoot)
	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("GET /callback", handleCallbackGET)
	mux.HandleFunc("POST /callback", handleCallbackPOST)
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

	mux.HandleFunc("GET /guild/{id}/sandbox", handleSandbox(client))
	mux.HandleFunc("POST /guild/{id}/sandbox/send", handleSandboxSend(client))

	// Static files.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(getStaticFS())))

	// Session cleanup ticker.
	exchangeCodeLimiter := newIPRateLimiter(
		rate.Every(time.Minute/exchangeCodeRatePerMinute),
		exchangeCodeBurst,
	)
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			model.CleanExpiredSessions()
			exchangeCodeLimiter.cleanup(rateLimiterTTL)
		}
	}()

	// Middleware chain: mux → auth → body limit → rate limit → CORS.
	withAuth := authMiddleware(mux)
	withBodyLimit := bodyLimitMiddleware(withAuth)
	withRateLimit := rateLimitMiddleware(exchangeCodeLimiter, "/callback")(withBodyLimit)
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{baseURL},
		AllowedMethods:   []string{"POST", "GET", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "HX-Request", "HX-Target", "HX-Current-URL"},
		AllowCredentials: true,
	}).Handler(withRateLimit)

	s := http.Server{
		Addr:              addr,
		Handler:           corsHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("Starting web server", "addr", addr)
	return s.ListenAndServe()
}

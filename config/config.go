package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/disgoorg/snowflake/v2"
	"github.com/spf13/viper"
)

// DashboardTokenKeyBytes is the required length of the AES-256-GCM key used
// to encrypt OAuth tokens at rest. Configured as a base64 string in
// dashboard.token_encryption_key.
const DashboardTokenKeyBytes = 32

// DiscordClientID returns the Discord application's OAuth2 client ID parsed
// as a snowflake. Returns 0 + error when the configured value is empty or
// not a valid snowflake — callers (web server startup) should fail fast.
func DiscordClientID() (snowflake.ID, error) {
	raw := strings.TrimSpace(viper.GetString("discord.client_id"))
	if raw == "" {
		return 0, errors.New("discord.client_id is required for OAuth login (set HEIMDALLR_DISCORD_CLIENT_ID or discord.client_id)")
	}
	id, err := snowflake.Parse(raw)
	if err != nil {
		return 0, fmt.Errorf("discord.client_id %q is not a valid snowflake: %w", raw, err)
	}
	return id, nil
}

// DiscordClientSecret returns the Discord application's OAuth2 client secret.
// Returns an error if unset — Heimdallr won't start without it because every
// admin path requires the OAuth handshake.
func DiscordClientSecret() (string, error) {
	v := strings.TrimSpace(viper.GetString("discord.client_secret"))
	if v == "" {
		return "", errors.New("discord.client_secret is required for OAuth login (set HEIMDALLR_DISCORD_CLIENT_SECRET or discord.client_secret)")
	}
	return v, nil
}

// DashboardTokenEncryptionKey decodes the base64 AES-256-GCM key used to
// encrypt user OAuth tokens at rest in the dashboard_sessions table. The
// key must decode to exactly DashboardTokenKeyBytes; anything else is a
// misconfiguration that would silently truncate or panic on use.
//
// Generate one with: `openssl rand -base64 32`.
func DashboardTokenEncryptionKey() ([]byte, error) {
	raw := strings.TrimSpace(viper.GetString("dashboard.token_encryption_key"))
	if raw == "" {
		return nil, errors.New("dashboard.token_encryption_key is required (generate with `openssl rand -base64 32` and set HEIMDALLR_DASHBOARD_TOKEN_ENCRYPTION_KEY)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Accept URL-safe base64 too — operators copying from web UIs
		// sometimes get the URL-safe variant without realizing.
		if k2, err2 := base64.URLEncoding.DecodeString(raw); err2 == nil {
			key = k2
		} else {
			return nil, fmt.Errorf("dashboard.token_encryption_key is not valid base64: %w", err)
		}
	}
	if len(key) != DashboardTokenKeyBytes {
		return nil, fmt.Errorf("dashboard.token_encryption_key must decode to %d bytes, got %d", DashboardTokenKeyBytes, len(key))
	}
	return key, nil
}

// ParsedDashboardBaseURL returns dashboard.base_url parsed and validated. The
// scheme must be http or https and the host must be non-empty — url.Parse
// alone accepts inputs like "" or "example.com" (no scheme), which would
// otherwise produce broken relative login links and a CORS allow-list that
// matches no browser-sent Origin.
//
// A non-empty path, query, or fragment is rejected: the web server registers
// its routes (/callback, /static/, /guild/{id}/...) at the root, and templates
// emit absolute root-relative links, so the app only works mounted at a host
// root. Sub-path deployment would silently produce broken login links.
func ParsedDashboardBaseURL() (*url.URL, error) {
	raw := viper.GetString("dashboard.base_url")
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid dashboard.base_url %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("dashboard.base_url %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("dashboard.base_url %q: host is empty", raw)
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("dashboard.base_url %q: must not include a path; the dashboard only supports being mounted at the host root", raw)
	}
	if u.RawQuery != "" {
		return nil, fmt.Errorf("dashboard.base_url %q: must not include a query string", raw)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("dashboard.base_url %q: must not include a fragment", raw)
	}
	return u, nil
}

func init() {
	viper.SetConfigName("config")
	viper.SetConfigType("toml")

	viper.SetEnvPrefix("heimdallr")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// CONFIG PATHS
	viper.AddConfigPath("/etc/heimdallr/")
	viper.AddConfigPath("$HOME/.heimdallr")
	viper.AddConfigPath("$HOME/.config/heimdallr/")
	viper.AddConfigPath("$XDG_CONFIG_HOME/heimdallr/")
	viper.AddConfigPath("./")

	// SET DEFAULTS
	viper.SetDefault("bot.token", "")
	viper.SetDefault("bot.db", "heimdallr.db")

	viper.SetDefault("loglevel", "info")

	viper.SetDefault("dev_mode.enabled", false)
	viper.SetDefault("dev_mode.guild_id", 0)

	viper.SetDefault("web.address", ":8484")
	viper.SetDefault("web.trusted_proxies", []string{})

	viper.SetDefault("dashboard.base_url", "http://localhost:8484")

	// OAuth2 login uses Discord's authorization-code flow against the bot's
	// own application credentials. client_id is public (it's in the
	// authorize URL); client_secret is sensitive — keep it out of VCS by
	// setting HEIMDALLR_DISCORD_CLIENT_SECRET in the environment. The
	// redirect URI is derived from dashboard.base_url + "/oauth/callback"
	// and must be registered on the application's Discord developer
	// portal page.
	viper.SetDefault("discord.client_id", "")
	viper.SetDefault("discord.client_secret", "")

	// AES-256-GCM key (base64) for encrypting OAuth tokens at rest in
	// dashboard_sessions. The DB row already contains the session-token
	// hash and user identifiers, so leaking the DB without this key still
	// exposes user IDs; but encrypting the access/refresh tokens
	// specifically keeps a DB compromise from handing out call-anything
	// bearer tokens to Discord on behalf of every signed-in admin.
	viper.SetDefault("dashboard.token_encryption_key", "")

	// Audit log retention ceilings, in days. Per-guild settings may LOWER
	// these (or set their own value) but never raise above them. 0 means
	// "no limit / forever" — guilds may set 0 only when the bot ceiling
	// is also 0. The pruner runs at audit_log.prune_interval_hours.
	viper.SetDefault("audit_log.message_retention_days", 14)
	viper.SetDefault("audit_log.member_retention_days", 90)
	viper.SetDefault("audit_log.guild_retention_days", 0)
	viper.SetDefault("audit_log.prune_interval_hours", 6)

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); ok {
			slog.Warn("Config file not found; ignore if set using env vars.")
		} else {
			slog.Error("Error reading config file.", "error", err)
			panic(err)
		}

	}
}

package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

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
	viper.SetDefault("bot.db", "heimdallr.db?journal_mode=WAL")

	viper.SetDefault("loglevel", "info")

	viper.SetDefault("dev_mode.enabled", false)
	viper.SetDefault("dev_mode.guild_id", 0)

	viper.SetDefault("web.address", ":8484")
	viper.SetDefault("web.trusted_proxies", []string{})

	viper.SetDefault("dashboard.base_url", "http://localhost:8484")

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

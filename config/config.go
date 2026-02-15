package config

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/spf13/viper"
)

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

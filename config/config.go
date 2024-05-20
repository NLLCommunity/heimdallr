package config

import (
	"errors"
	"github.com/spf13/viper"
	"log/slog"
)

func init() {
	viper.SetConfigName("config")
	viper.SetConfigType("toml")

	// CONFIG PATHS
	viper.AddConfigPath("/etc/heimdallr/")
	viper.AddConfigPath("$HOME/.heimdallr")
	viper.AddConfigPath("$HOME/.config/heimdallr/")
	viper.AddConfigPath("$XDG_CONFIG_HOME/heimdallr/")
	viper.AddConfigPath("./")

	// SET DEFAULTS
	viper.SetDefault("bot.token", "")
	viper.SetDefault("bot.db", "heimdallr.db")

	viper.SetDefault("dev_mode.enabled", false)
	viper.SetDefault("dev_mode.guild_id", 0)

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// Config file not found; ignore error if desired
			slog.Warn("Config file not found; using defaults.")
		} else {
			slog.Error("Error reading config file.", "error", err)
			panic(err)
		}

	}
}

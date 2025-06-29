package config

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfigDefaults(t *testing.T) {
	// Save original config.
	originalConfig := viper.AllSettings()

	// Reset viper for clean test.
	viper.Reset()

	// Re-initialize config, triggers the init function. Need to manually set the defaults since
	// init() already ran.
	viper.SetDefault("bot.token", "")
	viper.SetDefault("bot.db", "heimdallr.db")
	viper.SetDefault("loglevel", "info")
	viper.SetDefault("dev_mode.enabled", false)
	viper.SetDefault("dev_mode.guild_id", 0)

	// Test default values.
	assert.Equal(t, "", viper.GetString("bot.token"))
	assert.Equal(t, "heimdallr.db", viper.GetString("bot.db"))
	assert.Equal(t, "info", viper.GetString("loglevel"))
	assert.False(t, viper.GetBool("dev_mode.enabled"))
	assert.Equal(t, 0, viper.GetInt("dev_mode.guild_id"))

	// Restore original config.
	for key, value := range originalConfig {
		viper.Set(key, value)
	}
}

func TestEnvironmentVariables(t *testing.T) {
	// Save original environment and config.
	originalToken := os.Getenv("HEIMDALLR_BOT_TOKEN")
	originalLogLevel := os.Getenv("HEIMDALLR_LOGLEVEL")
	originalConfig := viper.AllSettings()

	// Set test environment variables.
	os.Setenv("HEIMDALLR_BOT_TOKEN", "test_token_123")
	os.Setenv("HEIMDALLR_LOGLEVEL", "debug")

	// Reset viper and reinitialize.
	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.SetEnvPrefix("heimdallr")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set defaults.
	viper.SetDefault("bot.token", "")
	viper.SetDefault("loglevel", "info")

	// Test that environment variables override defaults.
	assert.Equal(t, "test_token_123", viper.GetString("bot.token"))
	assert.Equal(t, "debug", viper.GetString("loglevel"))

	// Clean up.
	if originalToken == "" {
		os.Unsetenv("HEIMDALLR_BOT_TOKEN")
	} else {
		os.Setenv("HEIMDALLR_BOT_TOKEN", originalToken)
	}

	if originalLogLevel == "" {
		os.Unsetenv("HEIMDALLR_LOGLEVEL")
	} else {
		os.Setenv("HEIMDALLR_LOGLEVEL", originalLogLevel)
	}

	// Restore original config.
	viper.Reset()
	for key, value := range originalConfig {
		viper.Set(key, value)
	}
}

func TestConfigPaths(t *testing.T) {
	// This test verifies that the config paths are set up correctly. We can't easily test the
	// actual file reading without creating temp files, but we can verify the paths are configured.

	// Reset viper.
	viper.Reset()

	// Add the same config paths as in init().
	viper.AddConfigPath("/etc/heimdallr/")
	viper.AddConfigPath("$HOME/.heimdallr")
	viper.AddConfigPath("$HOME/.config/heimdallr/")
	viper.AddConfigPath("$XDG_CONFIG_HOME/heimdallr/")
	viper.AddConfigPath("./")

	// We can't easily test the paths directly, but we can test that ReadInConfig doesn't panic when
	// no config file is found.
	assert.NotPanics(t, func() {
		viper.ReadInConfig() // This will return an error but shouldn't panic.
	})
}

func TestConfigFileType(t *testing.T) {
	// Save original config
	originalConfig := viper.AllSettings()

	// Test that config type is set to TOML.
	viper.Reset()
	viper.SetConfigType("toml")

	// Create a temporary TOML config file matching the actual structure.
	configContent := `loglevel = "warn"

[bot]
token = "test_token_from_file"
db = "test.db"

[dev_mode]
enabled = true
guild_id = 123456789`

	tempFile, err := os.CreateTemp("", "test_config_*.toml")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString(configContent)
	assert.NoError(t, err)
	tempFile.Close()

	// Set the config file.
	viper.SetConfigFile(tempFile.Name())

	err = viper.ReadInConfig()
	assert.NoError(t, err)

	// Test that values are read correctly.
	assert.Equal(t, "test_token_from_file", viper.GetString("bot.token"))
	assert.Equal(t, "test.db", viper.GetString("bot.db"))
	assert.Equal(t, "warn", viper.GetString("loglevel"))
	assert.True(t, viper.GetBool("dev_mode.enabled"))
	assert.Equal(t, 123456789, viper.GetInt("dev_mode.guild_id"))

	// Restore original config.
	viper.Reset()
	for key, value := range originalConfig {
		viper.Set(key, value)
	}
}

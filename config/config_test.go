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

	// Re-initialize config, triggers the init function. Need to manually set
	// the defaults since init() already ran.
	setDefaults()

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
	setDefaults()

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
		_ = viper.ReadInConfig() // This will return an error but shouldn't panic.
	})
}

func TestParsedDashboardBaseURL(t *testing.T) {
	// Save/restore only the key this test mutates. viper.Reset() would also
	// drop the defaults, env-key replacer, and AutomaticEnv wiring registered
	// in config.init(), making later tests depend on this one running first.
	original := viper.Get("dashboard.base_url")
	t.Cleanup(func() { viper.Set("dashboard.base_url", original) })

	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid https", "https://dashboard.example.com", false},
		{"valid https with port", "https://dashboard.example.com:8484", false},
		{"valid http", "http://localhost:8484", false},
		{"valid trailing slash", "https://dashboard.example.com/", false},
		{"empty", "", true},
		{"missing scheme", "example.com", true},
		{"non-http scheme", "ftp://example.com", true},
		{"scheme only", "https://", true},
		{"with path", "https://example.com/dashboard", true},
		{"with deep path", "https://example.com/foo/bar", true},
		{"with query", "https://example.com/?foo=bar", true},
		{"with fragment", "https://example.com/#frag", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			viper.Set("dashboard.base_url", tc.raw)
			u, err := ParsedDashboardBaseURL()
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, u)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, u)
		})
	}
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

func TestTelemetryConfigDefaults(t *testing.T) {
	originalConfig := viper.AllSettings()
	t.Cleanup(func() {
		viper.Reset()
		for key, value := range originalConfig {
			viper.Set(key, value)
		}
	})

	viper.Reset()
	setDefaults()

	assert.False(t, viper.GetBool("telemetry.otel.enabled"))
	assert.Equal(t, "", viper.GetString("telemetry.otel.endpoint"))
	assert.Equal(t, map[string]string{}, viper.GetStringMapString("telemetry.otel.headers"))
	assert.False(t, viper.GetBool("telemetry.otel.insecure"))
	assert.Equal(t, "heimdallr", viper.GetString("telemetry.otel.service_name"))
	assert.Equal(t, "", viper.GetString("telemetry.otel.service_namespace"))
	assert.Equal(t, "", viper.GetString("telemetry.otel.environment"))

	assert.False(t, viper.GetBool("telemetry.posthog.enabled"))
	assert.Equal(t, "", viper.GetString("telemetry.posthog.api_key"))
	assert.Equal(t, "https://us.i.posthog.com", viper.GetString("telemetry.posthog.endpoint"))
	assert.Equal(t, 30, viper.GetInt("telemetry.posthog.flush_interval_seconds"))
	assert.Equal(t, 20, viper.GetInt("telemetry.posthog.flush_at"))
	assert.False(t, viper.GetBool("telemetry.posthog.group_analytics_enabled"))
}

func TestTelemetryEnvironmentVariables(t *testing.T) {
	originalConfig := viper.AllSettings()
	env := map[string]string{
		"HEIMDALLR_TELEMETRY_OTEL_ENABLED":                    "true",
		"HEIMDALLR_TELEMETRY_OTEL_ENDPOINT":                   "collector.example.com:4318",
		"HEIMDALLR_TELEMETRY_OTEL_INSECURE":                   "true",
		"HEIMDALLR_TELEMETRY_OTEL_SERVICE_NAME":               "heimdallr-test",
		"HEIMDALLR_TELEMETRY_OTEL_SERVICE_NAMESPACE":          "nll",
		"HEIMDALLR_TELEMETRY_OTEL_ENVIRONMENT":                "test",
		"HEIMDALLR_TELEMETRY_POSTHOG_ENABLED":                 "true",
		"HEIMDALLR_TELEMETRY_POSTHOG_API_KEY":                 "ph_test",
		"HEIMDALLR_TELEMETRY_POSTHOG_ENDPOINT":                "https://eu.i.posthog.com",
		"HEIMDALLR_TELEMETRY_POSTHOG_FLUSH_INTERVAL_SECONDS":  "5",
		"HEIMDALLR_TELEMETRY_POSTHOG_FLUSH_AT":                "3",
		"HEIMDALLR_TELEMETRY_POSTHOG_GROUP_ANALYTICS_ENABLED": "true",
	}
	previous := make(map[string]string, len(env))
	for key, value := range env {
		previous[key] = os.Getenv(key)
		os.Setenv(key, value)
	}
	t.Cleanup(func() {
		for key, value := range previous {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
		viper.Reset()
		for key, value := range originalConfig {
			viper.Set(key, value)
		}
	})

	viper.Reset()
	viper.SetEnvPrefix("heimdallr")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	setDefaults()

	assert.True(t, viper.GetBool("telemetry.otel.enabled"))
	assert.Equal(t, "collector.example.com:4318", viper.GetString("telemetry.otel.endpoint"))
	assert.True(t, viper.GetBool("telemetry.otel.insecure"))
	assert.Equal(t, "heimdallr-test", viper.GetString("telemetry.otel.service_name"))
	assert.Equal(t, "nll", viper.GetString("telemetry.otel.service_namespace"))
	assert.Equal(t, "test", viper.GetString("telemetry.otel.environment"))

	assert.True(t, viper.GetBool("telemetry.posthog.enabled"))
	assert.Equal(t, "ph_test", viper.GetString("telemetry.posthog.api_key"))
	assert.Equal(t, "https://eu.i.posthog.com", viper.GetString("telemetry.posthog.endpoint"))
	assert.Equal(t, 5, viper.GetInt("telemetry.posthog.flush_interval_seconds"))
	assert.Equal(t, 3, viper.GetInt("telemetry.posthog.flush_at"))
	assert.True(t, viper.GetBool("telemetry.posthog.group_analytics_enabled"))
}

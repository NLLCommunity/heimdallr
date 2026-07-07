package telemetry

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withCleanViper(t *testing.T) {
	t.Helper()
	original := viper.AllSettings()
	t.Cleanup(func() {
		viper.Reset()
		for key, value := range original {
			viper.Set(key, value)
		}
	})
	viper.Reset()
	viper.SetEnvPrefix("heimdallr")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	viper.SetDefault("telemetry.otel.enabled", false)
	viper.SetDefault("telemetry.otel.endpoint", "")
	viper.SetDefault("telemetry.otel.headers", map[string]string{})
	viper.SetDefault("telemetry.otel.insecure", false)
	viper.SetDefault("telemetry.otel.service_name", "heimdallr")
	viper.SetDefault("telemetry.otel.service_namespace", "")
	viper.SetDefault("telemetry.otel.environment", "")
	viper.SetDefault("telemetry.posthog.enabled", false)
	viper.SetDefault("telemetry.posthog.api_key", "")
	viper.SetDefault("telemetry.posthog.endpoint", "https://us.i.posthog.com")
	viper.SetDefault("telemetry.posthog.flush_interval_seconds", 30)
	viper.SetDefault("telemetry.posthog.flush_at", 20)
	viper.SetDefault("telemetry.posthog.group_analytics_enabled", false)
}

func TestConfigFromViperDisabledByDefault(t *testing.T) {
	withCleanViper(t)

	cfg := ConfigFromViper()

	assert.False(t, cfg.OTel.Enabled)
	assert.False(t, cfg.PostHog.Enabled)
	assert.Equal(t, "heimdallr", cfg.OTel.ServiceName)
	assert.Equal(t, "https://us.i.posthog.com", cfg.PostHog.Endpoint)
	assert.Equal(t, 30, cfg.PostHog.FlushIntervalSeconds)
	assert.Equal(t, 20, cfg.PostHog.FlushAt)
	require.NoError(t, cfg.Validate())
}

func TestConfigValidateAllowsMissingDisabledProviders(t *testing.T) {
	cfg := Config{}

	require.NoError(t, cfg.Validate())
}

func TestConfigValidateRequiresOTelEndpointWhenEnabled(t *testing.T) {
	cfg := Config{
		OTel: OTelConfig{
			Enabled:     true,
			ServiceName: "heimdallr",
		},
	}

	require.ErrorContains(t, cfg.Validate(), "telemetry.otel.endpoint is required")
}

func TestConfigValidateRequiresValidOTelEndpointWhenEnabled(t *testing.T) {
	cfg := Config{
		OTel: OTelConfig{
			Enabled:     true,
			Endpoint:    "://bad",
			ServiceName: "heimdallr",
		},
	}

	require.ErrorContains(t, cfg.Validate(), "telemetry.otel.endpoint must be a valid endpoint")
}

func TestConfigValidateRequiresPostHogAPIKeyWhenEnabled(t *testing.T) {
	cfg := Config{
		PostHog: PostHogConfig{
			Enabled:  true,
			Endpoint: "https://us.i.posthog.com",
		},
	}

	require.ErrorContains(t, cfg.Validate(), "telemetry.posthog.api_key is required")
}

func TestConfigValidateRequiresValidPostHogEndpointWhenEnabled(t *testing.T) {
	cfg := Config{
		PostHog: PostHogConfig{
			Enabled:              true,
			APIKey:               "phc_test",
			Endpoint:             "://bad",
			FlushIntervalSeconds: 30,
			FlushAt:              20,
		},
	}

	require.ErrorContains(t, cfg.Validate(), "telemetry.posthog.endpoint must be a valid http or https URL")
}

func TestConfigValidateIndependence(t *testing.T) {
	cfg := Config{
		OTel: OTelConfig{
			Enabled:     true,
			Endpoint:    "collector.example.com:4318",
			ServiceName: "heimdallr",
		},
		PostHog: PostHogConfig{
			Enabled:  false,
			Endpoint: "",
		},
	}

	require.NoError(t, cfg.Validate())
}

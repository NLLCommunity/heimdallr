package telemetry

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	OTel    OTelConfig
	PostHog PostHogConfig
}

type OTelConfig struct {
	Enabled          bool
	Endpoint         string
	Headers          map[string]string
	Insecure         bool
	ServiceName      string
	ServiceNamespace string
	Environment      string
}

type PostHogConfig struct {
	Enabled               bool
	APIKey                string
	Endpoint              string
	FlushIntervalSeconds  int
	FlushAt               int
	GroupAnalyticsEnabled bool
}

var bareOTelEndpointPattern = regexp.MustCompile(`^(?:[A-Za-z0-9.-]+|\[[0-9A-Fa-f:]+\])(?::[0-9]{1,5})?$`)

func ConfigFromViper() Config {
	return Config{
		OTel: OTelConfig{
			Enabled:          viper.GetBool("telemetry.otel.enabled"),
			Endpoint:         strings.TrimSpace(viper.GetString("telemetry.otel.endpoint")),
			Headers:          viper.GetStringMapString("telemetry.otel.headers"),
			Insecure:         viper.GetBool("telemetry.otel.insecure"),
			ServiceName:      strings.TrimSpace(viper.GetString("telemetry.otel.service_name")),
			ServiceNamespace: strings.TrimSpace(viper.GetString("telemetry.otel.service_namespace")),
			Environment:      strings.TrimSpace(viper.GetString("telemetry.otel.environment")),
		},
		PostHog: PostHogConfig{
			Enabled:               viper.GetBool("telemetry.posthog.enabled"),
			APIKey:                strings.TrimSpace(viper.GetString("telemetry.posthog.api_key")),
			Endpoint:              strings.TrimSpace(viper.GetString("telemetry.posthog.endpoint")),
			FlushIntervalSeconds:  viper.GetInt("telemetry.posthog.flush_interval_seconds"),
			FlushAt:               viper.GetInt("telemetry.posthog.flush_at"),
			GroupAnalyticsEnabled: viper.GetBool("telemetry.posthog.group_analytics_enabled"),
		},
	}
}

func (cfg Config) Validate() error {
	var errs []error

	if cfg.OTel.Enabled {
		if strings.TrimSpace(cfg.OTel.Endpoint) == "" {
			errs = append(errs, errors.New("telemetry.otel.endpoint is required when telemetry.otel.enabled is true"))
		} else if err := validateOTelEndpoint(cfg.OTel.Endpoint); err != nil {
			errs = append(errs, err)
		}
		if strings.TrimSpace(cfg.OTel.ServiceName) == "" {
			errs = append(errs, errors.New("telemetry.otel.service_name is required when telemetry.otel.enabled is true"))
		}
	}

	if cfg.PostHog.Enabled {
		if strings.TrimSpace(cfg.PostHog.APIKey) == "" {
			errs = append(errs, errors.New("telemetry.posthog.api_key is required when telemetry.posthog.enabled is true"))
		}
		if strings.TrimSpace(cfg.PostHog.Endpoint) == "" {
			errs = append(errs, errors.New("telemetry.posthog.endpoint is required when telemetry.posthog.enabled is true"))
		} else if err := validatePostHogEndpoint(cfg.PostHog.Endpoint); err != nil {
			errs = append(errs, err)
		}
		if cfg.PostHog.FlushIntervalSeconds <= 0 {
			errs = append(errs, fmt.Errorf("telemetry.posthog.flush_interval_seconds must be positive, got %d", cfg.PostHog.FlushIntervalSeconds))
		}
		if cfg.PostHog.FlushAt <= 0 {
			errs = append(errs, fmt.Errorf("telemetry.posthog.flush_at must be positive, got %d", cfg.PostHog.FlushAt))
		}
	}

	return errors.Join(errs...)
}

func validateOTelEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return errors.New("telemetry.otel.endpoint must be a valid endpoint when telemetry.otel.enabled is true")
	}
	if strings.ContainsAny(endpoint, " \r\n\t") {
		return errors.New("telemetry.otel.endpoint must be a valid endpoint when telemetry.otel.enabled is true")
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("telemetry.otel.endpoint must be a valid endpoint when telemetry.otel.enabled is true")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("telemetry.otel.endpoint must be a valid http or https endpoint when telemetry.otel.enabled is true")
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("telemetry.otel.endpoint must be a valid endpoint when telemetry.otel.enabled is true")
		}
		return nil
	}
	if bareOTelEndpointPattern.MatchString(endpoint) {
		return nil
	}

	return errors.New("telemetry.otel.endpoint must be a valid endpoint when telemetry.otel.enabled is true")
}

func validatePostHogEndpoint(endpoint string) error {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" {
		return errors.New("telemetry.posthog.endpoint must be a valid http or https URL when telemetry.posthog.enabled is true")
	}

	switch parsed.Scheme {
	case "http", "https":
		return nil
	default:
		return errors.New("telemetry.posthog.endpoint must be a valid http or https URL when telemetry.posthog.enabled is true")
	}
}

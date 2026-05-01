package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all runtime configuration for sqmeter-alpaca-safetymonitor.
// Settings are loaded from a JSON config file (see Load). The only
// environment variable that influences runtime behaviour is LOG_LEVEL, which
// is a process/logging concern and not a configuration file setting.
type Config struct {
	// ConfigVersion records which config schema version this file was written
	// with. Absent in older files (loads as 0, treated as pre-versioning).
	// The current version is 1. Future breaking schema changes will increment
	// this and may apply migrations on load.
	ConfigVersion        int      `json:"config_version,omitempty"`
	SQMeterBaseURL       string   `json:"SQMETER_BASE_URL"`
	AlpacaHTTPBind       string   `json:"ALPACA_HTTP_BIND"`
	AlpacaHTTPPort       int      `json:"ALPACA_HTTP_PORT"`
	AlpacaDiscoveryPort  int      `json:"ALPACA_DISCOVERY_PORT"`
	PollIntervalSeconds  int      `json:"POLL_INTERVAL_SECONDS"`
	StaleAfterSeconds    int      `json:"STALE_AFTER_SECONDS"`
	FailClosed           bool     `json:"FAIL_CLOSED"`
	ConnectedOnStartup   bool     `json:"CONNECTED_ON_STARTUP"`
	CloudCoverUnsafePct  float64  `json:"CLOUD_COVER_UNSAFE_PERCENT"`
	CloudCoverCautionPct float64  `json:"CLOUD_COVER_CAUTION_PERCENT"`
	RequireLightStatus   bool     `json:"REQUIRE_LIGHT_SENSOR_STATUS_OK"`
	RequireEnvStatus     bool     `json:"REQUIRE_ENVIRONMENT_STATUS_OK"`
	RequireIRStatus      bool     `json:"REQUIRE_IR_TEMPERATURE_STATUS_OK"`
	SQMMinSafe           *float64 `json:"SQM_MIN_SAFE,omitempty"`
	HumidityMaxSafe      *float64 `json:"HUMIDITY_MAX_SAFE,omitempty"`
	DewpointMarginMinC   *float64 `json:"DEWPOINT_MARGIN_MIN_C,omitempty"`
	ManualOverride       string   `json:"MANUAL_OVERRIDE"`
	LogLevel             string   `json:"LOG_LEVEL"`
}

// CurrentConfigVersion is the schema version written to new and updated config files.
// Older configs that predate this field load as version 0 in the raw JSON, but
// Load stamps them with the current version before returning.
const CurrentConfigVersion = 1

// Defaults returns a Config populated with safe, conservative defaults.
func Defaults() *Config {
	return &Config{
		ConfigVersion:        CurrentConfigVersion,
		SQMeterBaseURL:       "http://sqmeter.local",
		AlpacaHTTPBind:       "127.0.0.1",
		AlpacaHTTPPort:       11111,
		AlpacaDiscoveryPort:  32227,
		PollIntervalSeconds:  5,
		StaleAfterSeconds:    30,
		FailClosed:           true,
		ConnectedOnStartup:   true,
		CloudCoverUnsafePct:  80,
		CloudCoverCautionPct: 50,
		RequireLightStatus:   true,
		RequireEnvStatus:     true,
		RequireIRStatus:      true,
		ManualOverride:       "auto",
		LogLevel:             "info",
	}
}

// Load reads config from path (may be empty) and applies LOG_LEVEL from the
// environment if set. Returns an error if the file exists but is invalid.
//
// Config loading order: defaults -> config file -> validation.
// The only environment variable applied at runtime is LOG_LEVEL.
// All other settings must be set in the config file.
//
// Configs written before config_version was introduced load as version 0.
// They are valid and load without error; only the in-memory representation
// is updated to CurrentConfigVersion. Future schema migrations, if any, will
// key off the loaded version before stamping it.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	if path != "" {
		data, err := os.ReadFile(path) // #nosec G304 -- path comes from the --config CLI flag, which the operator controls
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file %q: %w", path, err)
		}
		if err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parsing config file %q: %w", path, err)
			}
		}
	}

	// Stamp the current version so callers always see a versioned config,
	// even when loading a pre-versioning file (config_version absent → 0).
	cfg.ConfigVersion = CurrentConfigVersion

	// LOG_LEVEL is the only env var applied at runtime; it is a
	// process/logging concern rather than an application setting.
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.SQMeterBaseURL == "" {
		return fmt.Errorf("SQMETER_BASE_URL is required")
	}
	switch cfg.ManualOverride {
	case "auto", "force_safe", "force_unsafe":
	default:
		return fmt.Errorf("MANUAL_OVERRIDE must be auto, force_safe, or force_unsafe; got %q", cfg.ManualOverride)
	}
	if cfg.AlpacaHTTPPort < 1 || cfg.AlpacaHTTPPort > 65535 {
		return fmt.Errorf("ALPACA_HTTP_PORT must be 1-65535, got %d", cfg.AlpacaHTTPPort)
	}
	if cfg.AlpacaDiscoveryPort < 1 || cfg.AlpacaDiscoveryPort > 65535 {
		return fmt.Errorf("ALPACA_DISCOVERY_PORT must be 1-65535, got %d", cfg.AlpacaDiscoveryPort)
	}
	if cfg.PollIntervalSeconds < 1 {
		return fmt.Errorf("POLL_INTERVAL_SECONDS must be >= 1")
	}
	if cfg.StaleAfterSeconds < cfg.PollIntervalSeconds {
		return fmt.Errorf("STALE_AFTER_SECONDS (%d) must be >= POLL_INTERVAL_SECONDS (%d)",
			cfg.StaleAfterSeconds, cfg.PollIntervalSeconds)
	}
	return nil
}

// RestartRequiredFields returns the config fields whose new values require a
// server restart before they take effect.
func RestartRequiredFields(old, new_ *Config) []string {
	fields := make([]string, 0, 3)
	if old.AlpacaHTTPBind != new_.AlpacaHTTPBind {
		fields = append(fields, "ALPACA_HTTP_BIND")
	}
	if old.AlpacaHTTPPort != new_.AlpacaHTTPPort {
		fields = append(fields, "ALPACA_HTTP_PORT")
	}
	if old.AlpacaDiscoveryPort != new_.AlpacaDiscoveryPort {
		fields = append(fields, "ALPACA_DISCOVERY_PORT")
	}
	return fields
}

// NeedsRestart returns true if changing from old to new requires a server restart.
func NeedsRestart(old, new_ *Config) bool {
	return len(RestartRequiredFields(old, new_)) > 0
}

// IsWideOpen returns true when the bind address is not loopback-only, meaning
// the service is reachable from the network.
func IsWideOpen(bind string) bool {
	return bind != "127.0.0.1" && bind != "localhost" && bind != "::1"
}

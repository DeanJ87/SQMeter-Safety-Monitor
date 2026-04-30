package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration. Fields use the same names as
// the corresponding environment variables for clarity.
type Config struct {
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

// Defaults returns a Config populated with safe, conservative defaults.
func Defaults() *Config {
	return &Config{
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

// Load reads config from path (may be empty), then applies environment
// variable overrides.  Returns an error if the file exists but is invalid.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file %q: %w", path, err)
		}
		if err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parsing config file %q: %w", path, err)
			}
		}
	}

	applyEnv(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	strEnv := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	intEnv := func(key string, dst *int) {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	floatEnv := func(key string, dst *float64) {
		if v := os.Getenv(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = f
			}
		}
	}
	floatPtrEnv := func(key string, dst **float64) {
		if v := os.Getenv(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = &f
			}
		}
	}
	boolEnv := func(key string, dst *bool) {
		if v := os.Getenv(key); v != "" {
			*dst = parseBool(v, *dst)
		}
	}

	strEnv("SQMETER_BASE_URL", &cfg.SQMeterBaseURL)
	strEnv("ALPACA_HTTP_BIND", &cfg.AlpacaHTTPBind)
	intEnv("ALPACA_HTTP_PORT", &cfg.AlpacaHTTPPort)
	intEnv("ALPACA_DISCOVERY_PORT", &cfg.AlpacaDiscoveryPort)
	intEnv("POLL_INTERVAL_SECONDS", &cfg.PollIntervalSeconds)
	intEnv("STALE_AFTER_SECONDS", &cfg.StaleAfterSeconds)
	boolEnv("FAIL_CLOSED", &cfg.FailClosed)
	boolEnv("CONNECTED_ON_STARTUP", &cfg.ConnectedOnStartup)
	floatEnv("CLOUD_COVER_UNSAFE_PERCENT", &cfg.CloudCoverUnsafePct)
	floatEnv("CLOUD_COVER_CAUTION_PERCENT", &cfg.CloudCoverCautionPct)
	boolEnv("REQUIRE_LIGHT_SENSOR_STATUS_OK", &cfg.RequireLightStatus)
	boolEnv("REQUIRE_ENVIRONMENT_STATUS_OK", &cfg.RequireEnvStatus)
	boolEnv("REQUIRE_IR_TEMPERATURE_STATUS_OK", &cfg.RequireIRStatus)
	floatPtrEnv("SQM_MIN_SAFE", &cfg.SQMMinSafe)
	floatPtrEnv("HUMIDITY_MAX_SAFE", &cfg.HumidityMaxSafe)
	floatPtrEnv("DEWPOINT_MARGIN_MIN_C", &cfg.DewpointMarginMinC)
	strEnv("MANUAL_OVERRIDE", &cfg.ManualOverride)
	strEnv("LOG_LEVEL", &cfg.LogLevel)
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
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

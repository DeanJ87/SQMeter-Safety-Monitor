package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"sqmeter-ascom-alpaca/internal/config"
)

func TestDefaults(t *testing.T) {
	cfg := config.Defaults()
	if cfg.AlpacaHTTPPort != 11111 {
		t.Errorf("default HTTP port: want 11111, got %d", cfg.AlpacaHTTPPort)
	}
	if cfg.AlpacaDiscoveryPort != 32227 {
		t.Errorf("default discovery port: want 32227, got %d", cfg.AlpacaDiscoveryPort)
	}
	if !cfg.FailClosed {
		t.Error("FailClosed should default to true")
	}
	if cfg.ManualOverride != "auto" {
		t.Errorf("ManualOverride default: want auto, got %q", cfg.ManualOverride)
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load with empty path: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoad_JSONFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"SQMETER_BASE_URL":"http://test.local","ALPACA_HTTP_PORT":12345,"FAIL_CLOSED":false}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SQMeterBaseURL != "http://test.local" {
		t.Errorf("SQMeterBaseURL: want http://test.local, got %q", cfg.SQMeterBaseURL)
	}
	if cfg.AlpacaHTTPPort != 12345 {
		t.Errorf("AlpacaHTTPPort: want 12345, got %d", cfg.AlpacaHTTPPort)
	}
	if cfg.FailClosed {
		t.Error("FailClosed should be false from JSON")
	}
}

// TestLoad_ConfigFileIsSourceOfTruth verifies that all settings come from the
// config file and that no application-setting env vars silently override them.
func TestLoad_ConfigFileIsSourceOfTruth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"SQMETER_BASE_URL": "http://file.local",
		"ALPACA_HTTP_PORT": 22222,
		"MANUAL_OVERRIDE": "auto"
	}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SQMeterBaseURL != "http://file.local" {
		t.Errorf("SQMeterBaseURL: want http://file.local from config file, got %q", cfg.SQMeterBaseURL)
	}
	if cfg.AlpacaHTTPPort != 22222 {
		t.Errorf("AlpacaHTTPPort: want 22222 from config file, got %d", cfg.AlpacaHTTPPort)
	}
}

// TestLoad_LogLevelEnvOverride verifies LOG_LEVEL env var is still honoured
// (it is a process/logging concern, not an application setting).
func TestLoad_LogLevelEnvOverride(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: want debug from LOG_LEVEL env, got %q", cfg.LogLevel)
	}
}

// TestLoad_LogLevelEnvDoesNotAffectOtherFields verifies that setting LOG_LEVEL
// only changes LogLevel and leaves everything else at defaults.
func TestLoad_LogLevelEnvDoesNotAffectOtherFields(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defaults := config.Defaults()
	if cfg.AlpacaHTTPPort != defaults.AlpacaHTTPPort {
		t.Errorf("AlpacaHTTPPort should be default %d, got %d", defaults.AlpacaHTTPPort, cfg.AlpacaHTTPPort)
	}
	if cfg.SQMeterBaseURL != defaults.SQMeterBaseURL {
		t.Errorf("SQMeterBaseURL should be default %q, got %q", defaults.SQMeterBaseURL, cfg.SQMeterBaseURL)
	}
}

func TestLoad_InvalidManualOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":"http://test.local","MANUAL_OVERRIDE":"bad_value"}`), 0600)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for invalid MANUAL_OVERRIDE")
	}
}

func TestLoad_MissingURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Write a config that clears the URL
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":""}`), 0600)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for empty SQMETER_BASE_URL")
	}
}

// TestLoad_OldConfigWithoutVersionStillLoads ensures pre-versioning config
// files (no config_version field) are accepted and loaded as the current version.
func TestLoad_OldConfigWithoutVersionStillLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Simulate a config written before config_version was introduced.
	old := `{"SQMETER_BASE_URL":"http://old.local","ALPACA_HTTP_PORT":11111}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("old config should load without error: %v", err)
	}
	if cfg.SQMeterBaseURL != "http://old.local" {
		t.Errorf("SQMeterBaseURL: want http://old.local, got %q", cfg.SQMeterBaseURL)
	}
	if cfg.ConfigVersion != config.CurrentConfigVersion {
		t.Errorf("ConfigVersion: want %d (current), got %d", config.CurrentConfigVersion, cfg.ConfigVersion)
	}
}

// TestDefaults_HasCurrentConfigVersion ensures freshly-written configs always
// include the current schema version.
func TestDefaults_HasCurrentConfigVersion(t *testing.T) {
	cfg := config.Defaults()
	if cfg.ConfigVersion != config.CurrentConfigVersion {
		t.Errorf("Defaults() ConfigVersion: want %d, got %d", config.CurrentConfigVersion, cfg.ConfigVersion)
	}
}

// TestLoad_ConfigVersionWrittenToNewFile verifies SaveDefault produces a file
// that, when loaded, carries the current config version.
func TestLoad_ConfigVersionWrittenToNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.SaveDefault(path); err != nil {
		t.Fatalf("SaveDefault: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigVersion != config.CurrentConfigVersion {
		t.Errorf("config_version in saved file: want %d, got %d", config.CurrentConfigVersion, cfg.ConfigVersion)
	}
}

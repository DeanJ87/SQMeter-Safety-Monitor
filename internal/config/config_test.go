package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"sqmeter-alpaca-safetymonitor/internal/config"
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
		t.Error("FAIL_CLOSED should default to true")
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

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("SQMETER_BASE_URL", "http://override.local")
	t.Setenv("ALPACA_HTTP_PORT", "9999")
	t.Setenv("MANUAL_OVERRIDE", "force_unsafe")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SQMeterBaseURL != "http://override.local" {
		t.Errorf("env override: want http://override.local, got %q", cfg.SQMeterBaseURL)
	}
	if cfg.AlpacaHTTPPort != 9999 {
		t.Errorf("env port: want 9999, got %d", cfg.AlpacaHTTPPort)
	}
	if cfg.ManualOverride != "force_unsafe" {
		t.Errorf("env override mode: want force_unsafe, got %q", cfg.ManualOverride)
	}
}

func TestLoad_InvalidManualOverride(t *testing.T) {
	t.Setenv("MANUAL_OVERRIDE", "bad_value")
	_, err := config.Load("")
	if err == nil {
		t.Error("expected error for invalid MANUAL_OVERRIDE")
	}
}

func TestLoad_MissingURL(t *testing.T) {
	t.Setenv("SQMETER_BASE_URL", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Write a config that clears the URL
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":""}`), 0600)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for empty SQMETER_BASE_URL")
	}
}

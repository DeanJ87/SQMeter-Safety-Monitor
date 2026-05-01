package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sqmeter-alpaca-safetymonitor/internal/config"
)

// ---------- NeedsRestart -----------------------------------------------------

func TestNeedsRestart_SameConfig(t *testing.T) {
	a := config.Defaults()
	b := *a
	if config.NeedsRestart(a, &b) {
		t.Error("identical config should not require restart")
	}
}

func TestNeedsRestart_URLChange(t *testing.T) {
	a := config.Defaults()
	b := *a
	b.SQMeterBaseURL = "http://other.local"
	if config.NeedsRestart(a, &b) {
		t.Error("URL-only change should not require restart")
	}
}

func TestNeedsRestart_HTTPPortChange(t *testing.T) {
	a := config.Defaults()
	b := *a
	b.AlpacaHTTPPort = 22222
	if !config.NeedsRestart(a, &b) {
		t.Error("HTTP port change should require restart")
	}
}

func TestNeedsRestart_DiscoveryPortChange(t *testing.T) {
	a := config.Defaults()
	b := *a
	b.AlpacaDiscoveryPort = 33333
	if !config.NeedsRestart(a, &b) {
		t.Error("discovery port change should require restart")
	}
}

func TestNeedsRestart_BindChange(t *testing.T) {
	a := config.Defaults()
	b := *a
	b.AlpacaHTTPBind = "0.0.0.0"
	if !config.NeedsRestart(a, &b) {
		t.Error("bind address change should require restart")
	}
}

func TestRestartRequiredFields_NamesChangedRestartFields(t *testing.T) {
	a := config.Defaults()
	b := *a
	b.AlpacaHTTPBind = "0.0.0.0"
	b.AlpacaHTTPPort = 22222
	b.AlpacaDiscoveryPort = 33333

	got := config.RestartRequiredFields(a, &b)
	want := []string{"ALPACA_HTTP_BIND", "ALPACA_HTTP_PORT", "ALPACA_DISCOVERY_PORT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RestartRequiredFields() = %#v, want %#v", got, want)
	}
}

func TestRestartRequiredFields_IgnoresHotReloadableFields(t *testing.T) {
	a := config.Defaults()
	b := *a
	b.SQMeterBaseURL = "http://other.local"
	b.LogLevel = "debug"

	got := config.RestartRequiredFields(a, &b)
	if len(got) != 0 {
		t.Fatalf("RestartRequiredFields() = %#v, want empty", got)
	}
}

// ---------- IsWideOpen -------------------------------------------------------

func TestIsWideOpen_Loopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "localhost", "::1"} {
		if config.IsWideOpen(addr) {
			t.Errorf("IsWideOpen(%q) should be false (loopback)", addr)
		}
	}
}

func TestIsWideOpen_NonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0", "192.168.1.1", ""} {
		if !config.IsWideOpen(addr) {
			t.Errorf("IsWideOpen(%q) should be true", addr)
		}
	}
}

// ---------- Holder -----------------------------------------------------------

func TestHolder_Get_ReturnsInitialDefaults(t *testing.T) {
	h := config.NewHolder(config.Defaults(), "")
	cfg := h.Get()
	if cfg.AlpacaHTTPPort != 11111 {
		t.Errorf("want port 11111, got %d", cfg.AlpacaHTTPPort)
	}
}

func TestHolder_Path(t *testing.T) {
	h := config.NewHolder(config.Defaults(), "/tmp/test.json")
	if h.Path() != "/tmp/test.json" {
		t.Errorf("Path(): want /tmp/test.json, got %q", h.Path())
	}
}

func TestHolder_Update_Valid(t *testing.T) {
	h := config.NewHolder(config.Defaults(), "")
	updated := *h.Get()
	updated.LogLevel = "debug"
	if err := h.Update(&updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if h.Get().LogLevel != "debug" {
		t.Errorf("LogLevel not updated: got %q", h.Get().LogLevel)
	}
}

func TestHolder_Update_InvalidRejected(t *testing.T) {
	h := config.NewHolder(config.Defaults(), "")
	original := h.Get().LogLevel
	invalid := *h.Get()
	invalid.ManualOverride = "not_valid"
	if err := h.Update(&invalid); err == nil {
		t.Error("expected error for invalid ManualOverride")
	}
	if h.Get().LogLevel != original {
		t.Error("config should not be mutated after a rejected update")
	}
}

func TestHolder_Update_PersistsToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	h := config.NewHolder(config.Defaults(), path)

	updated := *h.Get()
	updated.LogLevel = "warn"
	if err := h.Update(&updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(string(data), "warn") {
		t.Errorf("expected 'warn' in persisted config, got: %s", data)
	}
}

func TestHolder_Update_NoPersistWhenPathEmpty(t *testing.T) {
	h := config.NewHolder(config.Defaults(), "")
	updated := *h.Get()
	updated.LogLevel = "debug"
	// Should not error even with no path
	if err := h.Update(&updated); err != nil {
		t.Fatalf("Update with no path: %v", err)
	}
}

func TestSaveDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.json")
	if err := config.SaveDefault(path); err != nil {
		t.Fatalf("SaveDefault: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(data), "sqmeter.local") {
		t.Errorf("expected default SQMeter URL in output, got: %s", data)
	}
}

// ---------- validate edge cases ----------------------------------------------

func TestLoad_InvalidPortRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":"http://test.local","ALPACA_HTTP_PORT":99999}`), 0600)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for port 99999 (out of range)")
	}
}

func TestLoad_ZeroPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":"http://test.local","ALPACA_HTTP_PORT":0}`), 0600)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for port 0")
	}
}

func TestLoad_StaleAfterLessThanPollInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":"http://test.local","POLL_INTERVAL_SECONDS":60,"STALE_AFTER_SECONDS":10}`), 0600)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error when STALE_AFTER_SECONDS < POLL_INTERVAL_SECONDS")
	}
}

func TestLoad_BoolFieldsFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// FailClosed = false via config file
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":"http://test.local","FAIL_CLOSED":false}`), 0600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FailClosed {
		t.Error("FailClosed should be false when set in config file")
	}

	// FailClosed = true via config file
	os.WriteFile(path, []byte(`{"SQMETER_BASE_URL":"http://test.local","FAIL_CLOSED":true}`), 0600)
	cfg, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.FailClosed {
		t.Error("FailClosed should be true when set in config file")
	}
}

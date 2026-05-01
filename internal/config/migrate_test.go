package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqmeter-alpaca-safetymonitor/internal/config"
)

// ---------- Load: version rejection ------------------------------------------

// TestLoad_FutureVersionFails verifies that a config_version newer than the
// binary supports produces a clear error rather than silently loading.
func TestLoad_FutureVersionFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	futureVersion := config.CurrentConfigVersion + 1
	content := []byte(`{"SQMETER_BASE_URL":"http://test.local","config_version":` +
		string(rune('0'+futureVersion)) + `}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for future config_version, got nil")
	}
	if !strings.Contains(err.Error(), "newer than this binary supports") {
		t.Errorf("error message should mention 'newer than this binary supports', got: %v", err)
	}
}

// TestLoad_CurrentVersionLoadsOK confirms the current version loads without error.
func TestLoad_CurrentVersionLoadsOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.SaveDefault(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("current version should load without error: %v", err)
	}
	if cfg.ConfigVersion != config.CurrentConfigVersion {
		t.Errorf("ConfigVersion: want %d, got %d", config.CurrentConfigVersion, cfg.ConfigVersion)
	}
}

// ---------- BackupConfigFile -------------------------------------------------

// TestBackupConfigFile verifies that a timestamped backup is created beside the
// original file with .bak extension and identical content.
func TestBackupConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := []byte(`{"SQMETER_BASE_URL":"http://original.local"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}

	backupPath, err := config.BackupConfigFile(path)
	if err != nil {
		t.Fatalf("BackupConfigFile: %v", err)
	}
	if backupPath == "" {
		t.Fatal("expected non-empty backup path")
	}
	if !strings.HasSuffix(backupPath, ".bak") {
		t.Errorf("backup path should end in .bak, got %q", backupPath)
	}
	if filepath.Dir(backupPath) != dir {
		t.Errorf("backup should be in same directory as original: want %q, got %q", dir, filepath.Dir(backupPath))
	}

	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("backup content mismatch: want %q, got %q", original, got)
	}
}

// TestBackupConfigFile_MissingSourceFails verifies that backing up a
// non-existent file returns an error (not a silent no-op).
func TestBackupConfigFile_MissingSourceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	_, err := config.BackupConfigFile(path)
	if err == nil {
		t.Fatal("expected error when source file does not exist")
	}
}

// ---------- PersistMigrationIfNeeded -----------------------------------------

// TestPersistMigrationIfNeeded_OldVersionBacksUpAndWrites verifies that when
// the on-disk config has an old version, PersistMigrationIfNeeded creates a
// backup and rewrites the file with the migrated (current-version) config.
func TestPersistMigrationIfNeeded_OldVersionBacksUpAndWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write a v0 config (no config_version field).
	old := `{"SQMETER_BASE_URL":"http://old.local","ALPACA_HTTP_PORT":11111}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	backupPath, err := config.PersistMigrationIfNeeded(path, cfg)
	if err != nil {
		t.Fatalf("PersistMigrationIfNeeded: %v", err)
	}
	if backupPath == "" {
		t.Fatal("expected a backup path to be returned")
	}

	// Backup must contain the original content.
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if !strings.Contains(string(backupData), "http://old.local") {
		t.Errorf("backup should preserve original URL, got: %s", backupData)
	}

	// Migrated file must be valid JSON with the current version.
	migratedData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migrated config: %v", err)
	}
	var migrated config.Config
	if err := json.Unmarshal(migratedData, &migrated); err != nil {
		t.Fatalf("migrated config is not valid JSON: %v", err)
	}
	if migrated.ConfigVersion != config.CurrentConfigVersion {
		t.Errorf("migrated config_version: want %d, got %d", config.CurrentConfigVersion, migrated.ConfigVersion)
	}
	if migrated.SQMeterBaseURL != "http://old.local" {
		t.Errorf("SQMeterBaseURL not preserved after migration: got %q", migrated.SQMeterBaseURL)
	}
}

// TestPersistMigrationIfNeeded_AlreadyCurrent_NoBackup verifies that a config
// already at the current version is not backed up or rewritten.
func TestPersistMigrationIfNeeded_AlreadyCurrent_NoBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.SaveDefault(path); err != nil {
		t.Fatal(err)
	}

	modTimeBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	backupPath, err := config.PersistMigrationIfNeeded(path, cfg)
	if err != nil {
		t.Fatalf("PersistMigrationIfNeeded: %v", err)
	}
	if backupPath != "" {
		t.Errorf("expected no backup for current-version config, got %q", backupPath)
	}

	modTimeAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !modTimeAfter.ModTime().Equal(modTimeBefore.ModTime()) {
		t.Error("config file should not be rewritten when already at current version")
	}
}

// TestPersistMigrationIfNeeded_EmptyPath_NoOp verifies that an empty path
// is a safe no-op.
func TestPersistMigrationIfNeeded_EmptyPath_NoOp(t *testing.T) {
	cfg := config.Defaults()
	backupPath, err := config.PersistMigrationIfNeeded("", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backupPath != "" {
		t.Errorf("expected empty backup path for empty config path, got %q", backupPath)
	}
}

// TestPersistMigrationIfNeeded_MissingFile_NoOp verifies that a missing config
// file is treated as a no-op (not an error).
func TestPersistMigrationIfNeeded_MissingFile_NoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	cfg := config.Defaults()
	backupPath, err := config.PersistMigrationIfNeeded(path, cfg)
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if backupPath != "" {
		t.Errorf("expected empty backup path when file does not exist, got %q", backupPath)
	}
}

// TestPersistMigrationIfNeeded_WritesValidJSON verifies that the migrated file
// round-trips through JSON without data loss.
func TestPersistMigrationIfNeeded_WritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	old := `{"SQMETER_BASE_URL":"http://roundtrip.local","ALPACA_HTTP_PORT":22222,"FAIL_CLOSED":false}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := config.PersistMigrationIfNeeded(path, cfg); err != nil {
		t.Fatalf("PersistMigrationIfNeeded: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migrated config: %v", err)
	}
	var out config.Config
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("migrated file is not valid JSON: %v", err)
	}
	if out.AlpacaHTTPPort != 22222 {
		t.Errorf("AlpacaHTTPPort not preserved: want 22222, got %d", out.AlpacaHTTPPort)
	}
	if out.FailClosed {
		t.Error("FailClosed should be false after migration round-trip")
	}
}

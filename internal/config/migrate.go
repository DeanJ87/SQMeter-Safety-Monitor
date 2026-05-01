package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// migrateConfig applies in-memory migrations from fromVersion to
// CurrentConfigVersion.  It is called by Load with the version read from the
// file before the current version is stamped.
//
// Migration table:
//
//	v0 → v1: config_version field introduced; no structural changes needed.
//
// To add a future migration, increment CurrentConfigVersion and add a case here.
func migrateConfig(cfg *Config, fromVersion int) error {
	if fromVersion > CurrentConfigVersion {
		return fmt.Errorf(
			"config_version %d is newer than this binary supports (%d); please upgrade the application",
			fromVersion, CurrentConfigVersion,
		)
	}
	// v0 → v1: config_version introduced; no field renames or removals.
	return nil
}

// BackupConfigFile copies path to a timestamped sibling file of the form
// <base>.YYYYMMDDTHHMMSSZ.bak and returns the backup file path.
func BackupConfigFile(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from operator-controlled --config flag
	if err != nil {
		return "", fmt.Errorf("reading config for backup: %w", err)
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	backupPath := fmt.Sprintf("%s.%s.bak", base, ts)
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("writing config backup %q: %w", backupPath, err)
	}
	return backupPath, nil
}

// PersistMigrationIfNeeded reads the current on-disk config version and, when
// it is older than CurrentConfigVersion, backs up the file and rewrites it
// with the already-migrated cfg.
//
// Returns the backup path when a migration write occurred; returns ("", nil)
// when no action was needed (file already current, or path is empty).
//
// Errors are non-fatal for callers: the migrated config is already in memory
// and the service can start normally even if the persist step fails.
func PersistMigrationIfNeeded(path string, cfg *Config) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- same path as Load
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading config for migration check: %w", err)
	}

	var raw struct {
		ConfigVersion int `json:"config_version"`
	}
	_ = json.Unmarshal(data, &raw) // on malformed JSON raw.ConfigVersion stays 0

	if raw.ConfigVersion >= CurrentConfigVersion {
		return "", nil
	}

	backupPath, err := BackupConfigFile(path)
	if err != nil {
		return "", fmt.Errorf("backing up config before migration persist: %w", err)
	}
	if err := saveToFile(cfg, path); err != nil {
		return backupPath, fmt.Errorf("writing migrated config: %w", err)
	}
	return backupPath, nil
}

package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// AppDataDirName is the subdirectory name used under %ProgramData% on Windows.
	AppDataDirName = "SQMeter ASCOM Alpaca"

	// LegacyAppDataDirName is the old subdirectory name used by beta releases.
	// On first startup after upgrade, files are copied from this directory to
	// the new AppDataDirName location.  The legacy directory is never deleted.
	LegacyAppDataDirName = "SQMeter SafetyMonitor"

	// ReleasesURL is the canonical URL for checking the latest release.
	ReleasesURL = "https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases"
)

// DefaultConfigPath returns the platform-appropriate default path for config.json.
//
// On Windows:  %ProgramData%\SQMeter ASCOM Alpaca\config.json
// Other:       <exeDir>/config.json  (preserves prior behaviour)
//
// The --config CLI flag always overrides this value.
func DefaultConfigPath(exeDir string) string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, AppDataDirName, "config.json")
		}
		// ProgramData is always set on modern Windows; fall back to exe dir.
	}
	return filepath.Join(exeDir, "config.json")
}

// DefaultUUIDPath returns the platform-appropriate default path for device-uuid.txt.
//
// On Windows:  %ProgramData%\SQMeter ASCOM Alpaca\device-uuid.txt
// Other:       <exeDir>/device-uuid.txt
func DefaultUUIDPath(exeDir string) string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, AppDataDirName, "device-uuid.txt")
		}
	}
	return filepath.Join(exeDir, "device-uuid.txt")
}

// DefaultOCUUIDPath returns the platform-appropriate default path for the
// ObservingConditions device UUID file.
//
// On Windows:  %ProgramData%\SQMeter ASCOM Alpaca\device-oc-uuid.txt
// Other:       <exeDir>/device-oc-uuid.txt
func DefaultOCUUIDPath(exeDir string) string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, AppDataDirName, "device-oc-uuid.txt")
		}
	}
	return filepath.Join(exeDir, "device-oc-uuid.txt")
}

// MigrateAppDataDir copies files from the legacy app data directory to the new
// one when an upgrade from the old "SQMeter SafetyMonitor" path is detected.
//
// Migration rules:
//   - If newDir already contains config.json, no action is taken (new wins).
//   - If newDir has no config.json and legacyDir contains config.json, all
//     files in legacyDir (config.json and any *.bak files) are copied to newDir.
//   - If neither directory exists, this is a fresh install; no action is taken.
//   - The legacy directory is never deleted or modified.
//   - If both directories exist but newDir has no config.json, the legacy files
//     are copied in.
//
// logger may be nil; informational messages are omitted in that case.
func MigrateAppDataDir(newDir, legacyDir string, logger *slog.Logger) error {
	newConfig := filepath.Join(newDir, "config.json")
	legacyConfig := filepath.Join(legacyDir, "config.json")

	// New config already exists — nothing to do.
	if _, err := os.Stat(newConfig); err == nil {
		if _, lerr := os.Stat(legacyDir); lerr == nil {
			if logger != nil {
				logger.Info("app data migration: new path already has config, legacy path ignored",
					"new", newDir, "legacy", legacyDir)
			}
		}
		return nil
	}

	// Legacy config does not exist — first install, nothing to migrate.
	if _, err := os.Stat(legacyConfig); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checking legacy config: %w", err)
	}

	// Legacy config exists and new config does not — perform migration.
	if err := os.MkdirAll(newDir, 0700); err != nil {
		return fmt.Errorf("creating new app data directory %q: %w", newDir, err)
	}

	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return fmt.Errorf("reading legacy directory %q: %w", legacyDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Copy config.json and any backup files (*.bak).
		if name != "config.json" && filepath.Ext(name) != ".bak" {
			continue
		}
		src := filepath.Join(legacyDir, name)
		dst := filepath.Join(newDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("migrating %q: %w", name, err)
		}
		if logger != nil {
			logger.Info("app data migration: copied file", "file", name, "from", legacyDir, "to", newDir)
		}
	}

	if logger != nil {
		logger.Info("app data migration complete", "new", newDir, "legacy", legacyDir,
			"note", "legacy directory preserved for rollback")
	}
	return nil
}

// copyFile copies a single file from src to dst, preserving permissions 0600.
// dst is created or overwritten.
func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is derived from operator-controlled ProgramData path
	if err != nil {
		return fmt.Errorf("opening source %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating destination %q: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying %q → %q: %w", src, dst, err)
	}
	return out.Close()
}

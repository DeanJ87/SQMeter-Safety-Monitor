package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	// AppDataDirName is the subdirectory name used under %ProgramData% on Windows.
	AppDataDirName = "SQMeter SafetyMonitor"

	// ReleasesURL is the canonical URL for checking the latest release.
	// NOTE: This URL targets the renamed repository (SQMeter-ASCOM-Alpaca).
	// Until the GitHub repo is actually renamed, GitHub will redirect from the
	// old SQMeter-Safety-Monitor URL. Once renamed, no further change is needed.
	ReleasesURL = "https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases"
)

// DefaultConfigPath returns the platform-appropriate default path for config.json.
//
// On Windows:  %ProgramData%\SQMeter SafetyMonitor\config.json
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
// On Windows:  %ProgramData%\SQMeter SafetyMonitor\device-uuid.txt
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
// On Windows:  %ProgramData%\SQMeter SafetyMonitor\device-oc-uuid.txt
// Other:       <exeDir>/device-oc-uuid.txt
func DefaultOCUUIDPath(exeDir string) string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, AppDataDirName, "device-oc-uuid.txt")
		}
	}
	return filepath.Join(exeDir, "device-oc-uuid.txt")
}

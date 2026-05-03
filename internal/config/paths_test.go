package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sqmeter-alpaca-safetymonitor/internal/config"
)

// TestDefaultConfigPath_NonWindows verifies that on non-Windows platforms the
// default config path is beside the executable directory.
func TestDefaultConfigPath_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows path test skipped on Windows")
	}
	exeDir := "/opt/sqmeter"
	got := config.DefaultConfigPath(exeDir)
	want := filepath.Join(exeDir, "config.json")
	if got != want {
		t.Errorf("DefaultConfigPath(%q) = %q, want %q", exeDir, got, want)
	}
}

// TestDefaultUUIDPath_NonWindows verifies that on non-Windows the UUID file
// defaults to the executable directory.
func TestDefaultUUIDPath_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows path test skipped on Windows")
	}
	exeDir := "/opt/sqmeter"
	got := config.DefaultUUIDPath(exeDir)
	want := filepath.Join(exeDir, "device-uuid.txt")
	if got != want {
		t.Errorf("DefaultUUIDPath(%q) = %q, want %q", exeDir, got, want)
	}
}

// TestDefaultConfigPath_Windows_ProgramData verifies the Windows ProgramData
// path by injecting the env var (cross-platform test).
func TestDefaultConfigPath_Windows_ProgramData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows ProgramData path test only runs on Windows")
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		t.Skip("ProgramData not set")
	}
	got := config.DefaultConfigPath("/any/exe/dir")
	want := filepath.Join(pd, config.AppDataDirName, "config.json")
	if got != want {
		t.Errorf("DefaultConfigPath on Windows = %q, want %q", got, want)
	}
}

// TestDefaultUUIDPath_Windows_ProgramData verifies the Windows ProgramData
// UUID path.
func TestDefaultUUIDPath_Windows_ProgramData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows ProgramData path test only runs on Windows")
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		t.Skip("ProgramData not set")
	}
	got := config.DefaultUUIDPath("/any/exe/dir")
	want := filepath.Join(pd, config.AppDataDirName, "device-uuid.txt")
	if got != want {
		t.Errorf("DefaultUUIDPath on Windows = %q, want %q", got, want)
	}
}

// TestDefaultOCUUIDPath_NonWindows verifies that on non-Windows the OC UUID
// file defaults to the executable directory.
func TestDefaultOCUUIDPath_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows path test skipped on Windows")
	}
	exeDir := "/opt/sqmeter"
	got := config.DefaultOCUUIDPath(exeDir)
	want := filepath.Join(exeDir, "device-oc-uuid.txt")
	if got != want {
		t.Errorf("DefaultOCUUIDPath(%q) = %q, want %q", exeDir, got, want)
	}
}

// TestDefaultOCUUIDPath_Windows_ProgramData verifies the Windows ProgramData
// OC UUID path.
func TestDefaultOCUUIDPath_Windows_ProgramData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows ProgramData path test only runs on Windows")
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		t.Skip("ProgramData not set")
	}
	got := config.DefaultOCUUIDPath("/any/exe/dir")
	want := filepath.Join(pd, config.AppDataDirName, "device-oc-uuid.txt")
	if got != want {
		t.Errorf("DefaultOCUUIDPath on Windows = %q, want %q", got, want)
	}
}

// TestSaveDefault_CreatesParentDirs verifies that SaveDefault creates any
// missing parent directories (required for the ProgramData path on first install).
func TestSaveDefault_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "subdir", "config.json")

	if err := config.SaveDefault(path); err != nil {
		t.Fatalf("SaveDefault with deep path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

// TestHolderUpdate_CreatesParentDirs verifies that Holder.Update creates any
// missing parent directories before writing, so the app can safely write to
// a ProgramData path that was not pre-created.
func TestHolderUpdate_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "dir", "config.json")

	h := config.NewHolder(config.Defaults(), path)
	updated := *h.Get()
	updated.LogLevel = "debug"
	if err := h.Update(&updated); err != nil {
		t.Fatalf("Update with deep path: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file not created by Update: %v", err)
	}
	if !strings.Contains(string(data), "debug") {
		t.Errorf("expected 'debug' in persisted config, got: %s", data)
	}
}

// TestReleasesURL verifies the releases URL constant is set and looks reasonable.
func TestReleasesURL(t *testing.T) {
	url := config.ReleasesURL
	if url == "" {
		t.Fatal("ReleasesURL must not be empty")
	}
	if !strings.HasPrefix(url, "https://github.com/") {
		t.Errorf("ReleasesURL should be a GitHub URL, got %q", url)
	}
}

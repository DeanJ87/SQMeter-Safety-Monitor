package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"sqmeter-ascom-alpaca/internal/config"
)

// writeConfig is a helper that creates a config.json in dir with the given content.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// readFile is a helper that reads a file and returns its contents as a string.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return string(data)
}

// TestMigrateAppDataDir_NewPathHasConfig verifies that when the new path already
// contains a config.json, MigrateAppDataDir does nothing (new wins, legacy untouched).
func TestMigrateAppDataDir_NewPathHasConfig(t *testing.T) {
	root := t.TempDir()
	newDir := filepath.Join(root, "new")
	legacyDir := filepath.Join(root, "legacy")

	newContent := `{"SQMETER_BASE_URL":"http://new.local"}`
	legacyContent := `{"SQMETER_BASE_URL":"http://legacy.local"}`
	writeConfig(t, newDir, newContent)
	writeConfig(t, legacyDir, legacyContent)

	if err := config.MigrateAppDataDir(newDir, legacyDir, nil); err != nil {
		t.Fatalf("MigrateAppDataDir: %v", err)
	}

	// New config must be unchanged.
	if got := readFile(t, filepath.Join(newDir, "config.json")); got != newContent {
		t.Errorf("new config modified: want %q, got %q", newContent, got)
	}
	// Legacy config must be unchanged.
	if got := readFile(t, filepath.Join(legacyDir, "config.json")); got != legacyContent {
		t.Errorf("legacy config modified: want %q, got %q", legacyContent, got)
	}
}

// TestMigrateAppDataDir_LegacyOnly verifies that when only the legacy directory
// exists, config.json (and any .bak files) are copied to the new directory, and
// the legacy directory is left intact.
func TestMigrateAppDataDir_LegacyOnly(t *testing.T) {
	root := t.TempDir()
	newDir := filepath.Join(root, "new")
	legacyDir := filepath.Join(root, "legacy")

	legacyContent := `{"SQMETER_BASE_URL":"http://legacy.local"}`
	bakContent := `{"SQMETER_BASE_URL":"http://bak.local"}`
	writeConfig(t, legacyDir, legacyContent)
	// Also write a backup file.
	if err := os.WriteFile(filepath.Join(legacyDir, "config.20240101T000000Z.bak"), []byte(bakContent), 0600); err != nil {
		t.Fatal(err)
	}

	if err := config.MigrateAppDataDir(newDir, legacyDir, nil); err != nil {
		t.Fatalf("MigrateAppDataDir: %v", err)
	}

	// New config must contain the migrated content.
	if got := readFile(t, filepath.Join(newDir, "config.json")); got != legacyContent {
		t.Errorf("migrated config: want %q, got %q", legacyContent, got)
	}
	// Backup file must also have been copied.
	if got := readFile(t, filepath.Join(newDir, "config.20240101T000000Z.bak")); got != bakContent {
		t.Errorf("migrated backup: want %q, got %q", bakContent, got)
	}
	// Legacy directory must still exist and be unmodified.
	if got := readFile(t, filepath.Join(legacyDir, "config.json")); got != legacyContent {
		t.Errorf("legacy config was modified: want %q, got %q", legacyContent, got)
	}
}

// TestMigrateAppDataDir_BothExist_NewNoConfig verifies that when both directories
// exist but the new one lacks a config.json, the legacy config is copied in.
func TestMigrateAppDataDir_BothExist_NewNoConfig(t *testing.T) {
	root := t.TempDir()
	newDir := filepath.Join(root, "new")
	legacyDir := filepath.Join(root, "legacy")

	legacyContent := `{"SQMETER_BASE_URL":"http://legacy.local"}`
	writeConfig(t, legacyDir, legacyContent)
	// Create newDir but leave it empty (no config.json).
	if err := os.MkdirAll(newDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := config.MigrateAppDataDir(newDir, legacyDir, nil); err != nil {
		t.Fatalf("MigrateAppDataDir: %v", err)
	}

	// Migrated config must appear in new path.
	if got := readFile(t, filepath.Join(newDir, "config.json")); got != legacyContent {
		t.Errorf("migrated config: want %q, got %q", legacyContent, got)
	}
}

// TestMigrateAppDataDir_NeitherExists verifies that when neither directory exists
// (fresh install), MigrateAppDataDir is a no-op and returns no error.
func TestMigrateAppDataDir_NeitherExists(t *testing.T) {
	root := t.TempDir()
	newDir := filepath.Join(root, "new")
	legacyDir := filepath.Join(root, "legacy")

	if err := config.MigrateAppDataDir(newDir, legacyDir, nil); err != nil {
		t.Fatalf("MigrateAppDataDir on fresh install: %v", err)
	}
	// Neither directory should have been created.
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Errorf("new directory should not exist after no-op migration")
	}
}

// TestMigrateAppDataDir_UnwritableDest verifies that when the destination
// directory cannot be created, MigrateAppDataDir returns a clear error.
func TestMigrateAppDataDir_UnwritableDest(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}

	root := t.TempDir()
	legacyDir := filepath.Join(root, "legacy")
	legacyContent := `{"SQMETER_BASE_URL":"http://legacy.local"}`
	writeConfig(t, legacyDir, legacyContent)

	// Create a read-only parent so MkdirAll on newDir fails.
	readOnlyParent := filepath.Join(root, "readonly")
	if err := os.MkdirAll(readOnlyParent, 0500); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(readOnlyParent, "new")

	err := config.MigrateAppDataDir(newDir, legacyDir, nil)
	if err == nil {
		t.Fatal("expected error when destination is unwritable, got nil")
	}
}

// TestMigrateAppDataDir_SaveDefaultUsesNewPath verifies that SaveDefault writes
// to the new AppDataDirName path (not the legacy one).
func TestMigrateAppDataDir_SaveDefaultUsesNewPath(t *testing.T) {
	root := t.TempDir()
	newConfigPath := filepath.Join(root, config.AppDataDirName, "config.json")

	if err := config.SaveDefault(newConfigPath); err != nil {
		t.Fatalf("SaveDefault: %v", err)
	}
	if _, err := os.Stat(newConfigPath); err != nil {
		t.Fatalf("config not created at new path: %v", err)
	}
	// Verify AppDataDirName constant is the new value.
	if config.AppDataDirName == config.LegacyAppDataDirName {
		t.Error("AppDataDirName must differ from LegacyAppDataDirName")
	}
}

// TestMigrateAppDataDir_NewConfigNotOverwritten verifies that if the new config
// exists, it is not overwritten even if the legacy config has different content.
func TestMigrateAppDataDir_NewConfigNotOverwritten(t *testing.T) {
	root := t.TempDir()
	newDir := filepath.Join(root, "new")
	legacyDir := filepath.Join(root, "legacy")

	newContent := `{"SQMETER_BASE_URL":"http://new.local"}`
	legacyContent := `{"SQMETER_BASE_URL":"http://completely-different.local"}`
	writeConfig(t, newDir, newContent)
	writeConfig(t, legacyDir, legacyContent)

	if err := config.MigrateAppDataDir(newDir, legacyDir, nil); err != nil {
		t.Fatalf("MigrateAppDataDir: %v", err)
	}

	if got := readFile(t, filepath.Join(newDir, "config.json")); got != newContent {
		t.Errorf("new config was overwritten: want %q, got %q", newContent, got)
	}
}

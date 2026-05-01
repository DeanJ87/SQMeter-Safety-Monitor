package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Holder is a thread-safe, persistent wrapper around Config.
// Hot-reloadable fields take effect on the next poll or request cycle after
// Update is called; fields that require a server restart are noted in the web UI.
type Holder struct {
	mu   sync.RWMutex
	cfg  *Config
	path string // config file path; "" means no persistence
}

// NewHolder wraps cfg.  path is the file to which updates are persisted;
// pass "" to disable file persistence.
func NewHolder(cfg *Config, path string) *Holder {
	return &Holder{cfg: cfg, path: path}
}

// Get returns the current config.  The returned pointer must not be mutated.
func (h *Holder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Update validates newCfg, ensures the config directory exists, persists the
// config (if a path is set), then atomically replaces the current config.
func (h *Holder) Update(newCfg *Config) error {
	if err := validate(newCfg); err != nil {
		return err
	}
	if h.path != "" {
		if err := os.MkdirAll(filepath.Dir(h.path), 0700); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}
		if err := saveToFile(newCfg, h.path); err != nil {
			return err
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = newCfg
	return nil
}

// Path returns the config file path this holder is linked to.
func (h *Holder) Path() string { return h.path }

// SaveDefault writes the default config to path, creating parent directories
// as needed.  This is the correct way to initialise a config at a new location
// (e.g. %ProgramData%\SQMeter SafetyMonitor\config.json on first install).
func SaveDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return saveToFile(Defaults(), path)
}

func saveToFile(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

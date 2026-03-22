package agentcore

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const appliedConfigFile = "applied-config.json"

type appliedConfig struct {
	CollectIntervalSec   int `json:"collect_interval_sec,omitempty"`
	HeartbeatIntervalSec int `json:"heartbeat_interval_sec,omitempty"`
}

// persistAppliedConfig writes the current runtime overrides to disk.
func persistAppliedConfig(r *Runtime) {
	if r == nil {
		return
	}
	dir := configDir()
	if dir == "" {
		return
	}
	path := filepath.Join(dir, appliedConfigFile)
	cfg := appliedConfig{
		CollectIntervalSec:   int(r.collectIntervalOverride.Load()),
		HeartbeatIntervalSec: int(r.heartbeatIntervalOverride.Load()),
	}
	if cfg.CollectIntervalSec == 0 && cfg.HeartbeatIntervalSec == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("config: failed to remove applied config: %v", err)
		}
		return
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("config: failed to create config dir: %v", err)
		return
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Printf("config: failed to persist applied config: %v", err)
	}
}

// loadPersistedConfig reads applied-config.json and restores overrides to the runtime.
func loadPersistedConfig(r *Runtime) {
	if r == nil {
		return
	}
	dir := configDir()
	if dir == "" {
		return
	}
	path := filepath.Join(dir, appliedConfigFile)
	data, err := os.ReadFile(path) // #nosec G304 -- Path is the package-owned applied-config state file under the managed dir.
	if err != nil {
		return // file doesn't exist yet
	}
	var cfg appliedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("config: failed to parse applied config: %v", err)
		return
	}
	if cfg.CollectIntervalSec >= 2 && cfg.CollectIntervalSec <= 300 {
		r.collectIntervalOverride.Store(int64(cfg.CollectIntervalSec))
		r.mu.Lock()
		r.cfg.CollectInterval = time.Duration(cfg.CollectIntervalSec) * time.Second
		r.mu.Unlock()
		log.Printf("config: restored collect interval override: %ds", cfg.CollectIntervalSec)
	}
	if cfg.HeartbeatIntervalSec >= 5 && cfg.HeartbeatIntervalSec <= 600 {
		r.heartbeatIntervalOverride.Store(int64(cfg.HeartbeatIntervalSec))
		r.mu.Lock()
		r.cfg.HeartbeatInterval = time.Duration(cfg.HeartbeatIntervalSec) * time.Second
		r.mu.Unlock()
		log.Printf("config: restored heartbeat interval override: %ds", cfg.HeartbeatIntervalSec)
	}
}

// configDir returns the directory for persisted agent config.
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".labtether")
}

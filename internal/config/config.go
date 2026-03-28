package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	NodeID              string   `json:"node_id"`
	Address             string   `json:"address"`
	Peers               []string `json:"peers"`
	DataDir             string   `json:"data_dir"`
	ElectionTimeoutMinMs int     `json:"election_timeout_min_ms"`
	ElectionTimeoutMaxMs int     `json:"election_timeout_max_ms"`
	HeartbeatIntervalMs  int     `json:"heartbeat_interval_ms"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.ElectionTimeoutMinMs == 0 {
		cfg.ElectionTimeoutMinMs = 150
	}
	if cfg.ElectionTimeoutMaxMs == 0 {
		cfg.ElectionTimeoutMaxMs = 300
	}
	if cfg.HeartbeatIntervalMs == 0 {
		cfg.HeartbeatIntervalMs = 50
	}

	if cfg.NodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}
	if cfg.Address == "" {
		return nil, fmt.Errorf("address is required")
	}

	return &cfg, nil
}

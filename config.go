package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is the server configuration
type Config struct {
	VkToken         string `json:"vk_token"`
	PeerID          int    `json:"peer_id"`
	ThinkingPeerID  int    `json:"thinking_peer_id"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		VkToken: os.Getenv("VK_TOKEN"),
	}

	// Try loading from config files
	homeDir, _ := os.UserHomeDir()
	configPaths := []string{
		"./config.json",
		"/etc/mcp-vk-files/config.json",
	}
	if homeDir != "" {
		configPaths = append(configPaths, homeDir+"/.config/mcp-vk-files/config.json")
	}

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var fileCfg Config
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", path, err)
			continue
		}

		if fileCfg.VkToken != "" {
			cfg.VkToken = fileCfg.VkToken
		}
		if fileCfg.PeerID != 0 {
			cfg.PeerID = fileCfg.PeerID
		}
		if fileCfg.ThinkingPeerID != 0 {
			cfg.ThinkingPeerID = fileCfg.ThinkingPeerID
		}

		fmt.Fprintf(os.Stderr, "Config loaded from %s\n", path)
		break
	}

	if token := os.Getenv("VK_TOKEN"); token != "" {
		cfg.VkToken = token
	}

	if cfg.VkToken == "" {
		return nil, fmt.Errorf("VK_TOKEN is required")
	}

	return cfg, nil
}

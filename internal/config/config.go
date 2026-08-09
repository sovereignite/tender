package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration.
type Config struct {
	// GitHub App configuration
	GitHub GitHubConfig `json:"github"`

	// Libvirt configuration
	Libvirt LibvirtConfig `json:"libvirt"`

	// Runner defaults
	Runner RunnerConfig `json:"runner"`

	// Health check configuration
	Health HealthConfig `json:"health"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`
}

// GitHubConfig holds GitHub App configuration.
type GitHubConfig struct {
	AppID          int64  `json:"app_id"`
	PrivateKeyPath string `json:"private_key_path"`
	Organization   string `json:"organization"`
}

// LibvirtConfig holds libvirt connection configuration.
type LibvirtConfig struct {
	URI         string `json:"uri"`
	NetworkName string `json:"network_name"`
}

// RunnerConfig holds default runner configuration.
type RunnerConfig struct {
	Labels   []string `json:"labels"`
	Group    string   `json:"group"`
	MemoryMB uint     `json:"memory_mb"`
	CPUs     uint     `json:"cpus"`
}

// HealthConfig holds health check configuration.
type HealthConfig struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval"`
	Timeout  string `json:"timeout"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
	File   string `json:"file"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Libvirt: LibvirtConfig{
			URI:         "qemu:///system",
			NetworkName: "shuttle",
		},
		Runner: RunnerConfig{
			Labels:   []string{"self-hosted", "linux", "x64"},
			Group:    "default",
			MemoryMB: 4096,
			CPUs:     2,
		},
		Health: HealthConfig{
			Enabled:  true,
			Interval: "30s",
			Timeout:  "2m",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Load loads configuration from a file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Save saves configuration to a file.
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetDefaultConfigPath returns the default configuration file path.
func GetDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/etc/shuttle/config.json"
	}
	return filepath.Join(home, ".config", "shuttle", "config.json")
}

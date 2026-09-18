package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration parameters.
type Config struct {
	DBPath      string `yaml:"db_path"`
	StorageDir  string `yaml:"storage_dir"`
	EnableWAL   bool   `yaml:"enable_wal"`
	SwarmTopic  string `yaml:"swarm_topic"`
	LogLevel    string `yaml:"log_level"`
	MarkdownDir string `yaml:"markdown_dir"`
}

// DefaultConfig returns the default configuration options.
func DefaultConfig() Config {
	return Config{
		DBPath:      "crm.db",
		StorageDir:  ".crm_data",
		EnableWAL:   true,
		SwarmTopic:  "in.qzip.crm.cluster.v1",
		LogLevel:    "INFO",
		MarkdownDir: "records",
	}
}

// ResolveConfigPath checks configuration paths according to the 3-tier hierarchy:
// 1. Explicit path (e.g. from CLI flag)
// 2. Executable directory: <exe_dir>/config.yaml
// 3. Current working directory: ./config.yaml
// Returns the resolved file path, or empty string if no config file was found.
func ResolveConfigPath(explicitPath string) string {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err == nil {
			return explicitPath
		}
	}

	// Check executable directory
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		exeConfig := filepath.Join(exeDir, "config.yaml")
		if _, err := os.Stat(exeConfig); err == nil {
			return exeConfig
		}
	}

	// Check current directory
	cwdConfig := "config.yaml"
	if _, err := os.Stat(cwdConfig); err == nil {
		return cwdConfig
	}

	return ""
}

// Load loads configuration from the resolved path, falling back to defaults.
func Load(explicitPath string) (Config, error) {
	cfg := DefaultConfig()

	resolvedPath := ResolveConfigPath(explicitPath)
	if resolvedPath == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file at %s: %w", resolvedPath, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse yaml config at %s: %w", resolvedPath, err)
	}

	return cfg, nil
}

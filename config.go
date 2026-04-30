package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the user's universal settings
type Config struct {
	DefaultWorkspace string   `yaml:"default_workspace"`
	IgnoreList       []string `yaml:"ignore_directories"`
	MaxWorkers       int      `yaml:"max_workers"` // 0 means auto-calculate based on CPU
}

// LoadConfig fetches settings from the OS-aware config directory.
func LoadConfig() (Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, fmt.Errorf("could not find user config dir: %w", err)
	}

	appConfigDir := filepath.Join(configDir, "gitfleet")
	configPath := filepath.Join(appConfigDir, "config.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err := os.MkdirAll(appConfigDir, 0755)
		if err != nil {
			return Config{}, err
		}

		home, _ := os.UserHomeDir()
		defaultCfg := Config{
			DefaultWorkspace: filepath.Join(home, "clones"),
			IgnoreList:       []string{"node_modules", "target", "vendor", ".cache"},
			MaxWorkers:       0, // Auto-detect
		}

		data, err := yaml.Marshal(&defaultCfg)
		if err != nil {
			return Config{}, err
		}

		err = os.WriteFile(configPath, data, 0644)
		if err != nil {
			return Config{}, err
		}
		return defaultCfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, err
	}

	if cfg.DefaultWorkspace == "" {
		home, _ := os.UserHomeDir()
		cfg.DefaultWorkspace = filepath.Join(home, "clones")
	}

	return cfg, nil
}

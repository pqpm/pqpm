package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pqpm/pqpm/internal/types"
)

const ConfigFileName = ".pqpm.toml"

// LoadUserConfig reads and parses the .pqpm.toml file from the given home directory.
func LoadUserConfig(homeDir string) (*types.UserConfig, error) {
	configPath := filepath.Join(homeDir, ConfigFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg types.UserConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	if cfg.Service == nil {
		cfg.Service = make(map[string]types.ServiceConfig)
	}

	return &cfg, nil
}

// GetServiceConfig returns the config for a specific named service.
func GetServiceConfig(cfg *types.UserConfig, name string) (*types.ServiceConfig, error) {
	svc, ok := cfg.Service[name]
	if !ok {
		return nil, fmt.Errorf("service %q not found in config", name)
	}
	return &svc, nil
}

// ValidateServiceConfig performs basic validation on a service configuration.
func ValidateServiceConfig(name string, svc *types.ServiceConfig) error {
	if svc.Command == "" {
		return fmt.Errorf("service %q: command is required", name)
	}

	validRestart := map[string]bool{
		"always":     true,
		"on-failure": true,
		"never":      true,
		"":           true, // default to "always"
	}
	if !validRestart[svc.Restart] {
		return fmt.Errorf("service %q: invalid restart policy %q (must be always, on-failure, or never)", name, svc.Restart)
	}

	return nil
}

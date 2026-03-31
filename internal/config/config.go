package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

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

	// Basic validation for all services
	for name, svc := range cfg.Service {
		if err := ValidateServiceConfig(name, &svc); err != nil {
			return nil, err
		}
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

// ValidateServiceConfig performs security validation on a service configuration.
func ValidateServiceConfig(name string, svc *types.ServiceConfig) error {
	if svc.Command == "" {
		return fmt.Errorf("service %q: command is required", name)
	}

	// Prevent suspicious shell operators if the command contains them
	// Since we use strings.Fields to split, we don't use a shell by default,
	// but users might try to execute "sh -c ..."
	dangerousOperators := []string{";", "&&", "||", "|", ">", "<", "`", "$("}
	for _, op := range dangerousOperators {
		if strings.Contains(svc.Command, op) {
			return fmt.Errorf("service %q: command contains dangerous shell operator %q", name, op)
		}
	}

	// Validate working directory if provided
	if svc.WorkingDir != "" {
		if !filepath.IsAbs(svc.WorkingDir) {
			return fmt.Errorf("service %q: working_dir must be an absolute path", name)
		}
		// Ensure it exists
		if _, err := os.Stat(svc.WorkingDir); err != nil {
			return fmt.Errorf("service %q: working_dir %q does not exist", name, svc.WorkingDir)
		}
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

// SanitizeUserPath ensures that a given path is within the user's home directory.
// This is used as an additional security layer.
func SanitizeUserPath(path string, uid uint32) error {
	u, err := user.LookupId(fmt.Sprintf("%d", uid))
	if err != nil {
		return fmt.Errorf("failed to lookup user: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	if !strings.HasPrefix(absPath, u.HomeDir) {
		return fmt.Errorf("access denied: path %q is outside user home directory %q", absPath, u.HomeDir)
	}

	return nil
}

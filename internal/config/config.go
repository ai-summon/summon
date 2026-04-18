package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Platforms holds the enabled/disabled state for each known platform.
// Pointer fields distinguish "not configured" (nil) from "explicitly disabled" (false).
type Platforms struct {
	Copilot *bool `yaml:"copilot,omitempty"`
	Claude  *bool `yaml:"claude,omitempty"`
}

// Config represents the summon configuration file (~/.summon/config.yaml).
type Config struct {
	Platforms Platforms `yaml:"platforms"`
}

// KnownPlatforms returns the names of all platforms summon knows about.
func KnownPlatforms() []string {
	return []string{"claude", "copilot"}
}

// IsEnabled returns whether a platform is enabled and whether it was explicitly configured.
// For an unknown platform name, returns (false, false).
func (c *Config) IsEnabled(name string) (enabled bool, configured bool) {
	var ptr *bool
	switch name {
	case "copilot":
		ptr = c.Platforms.Copilot
	case "claude":
		ptr = c.Platforms.Claude
	default:
		return false, false
	}
	if ptr == nil {
		return false, false
	}
	return *ptr, true
}

// SetPlatform sets the enabled state for a named platform.
// Returns an error for unknown platform names.
func (c *Config) SetPlatform(name string, enabled bool) error {
	switch name {
	case "copilot":
		c.Platforms.Copilot = &enabled
	case "claude":
		c.Platforms.Claude = &enabled
	default:
		return fmt.Errorf("unknown platform %q; known platforms: %v", name, KnownPlatforms())
	}
	return nil
}

// HasPlatforms returns true if at least one platform is configured.
func (c *Config) HasPlatforms() bool {
	return c.Platforms.Copilot != nil || c.Platforms.Claude != nil
}

// EnabledPlatforms returns the names of all explicitly enabled platforms.
func (c *Config) EnabledPlatforms() []string {
	var result []string
	for _, name := range KnownPlatforms() {
		if enabled, configured := c.IsEnabled(name); configured && enabled {
			result = append(result, name)
		}
	}
	return result
}

// Load reads a Config from the given path.
// If the file does not exist, it returns a zero-value Config and no error.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes a Config to the given path atomically.
// It creates parent directories if needed.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Atomic write: write to temp file in same directory, then rename
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming config: %w", err)
	}
	return nil
}

// DefaultPath returns the default config file path: ~/.summon/config.yaml
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".summon", "config.yaml"), nil
}

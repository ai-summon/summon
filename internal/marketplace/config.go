package marketplace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds user marketplace registrations.
type Config struct {
	Marketplaces []MarketplaceEntry `yaml:"marketplaces"`
}

// MarketplaceEntry is a registered marketplace in user config.
type MarketplaceEntry struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
}

// DefaultConfigDir returns the default config directory (~/.summon).
func DefaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".summon")
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

// LoadConfig reads the user config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes the user config file, creating directories as needed.
func SaveConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// AddMarketplace adds a marketplace to the config.
func (c *Config) AddMarketplace(name, source string) error {
	for _, m := range c.Marketplaces {
		if m.Name == name {
			return fmt.Errorf("marketplace %q already registered", name)
		}
	}
	c.Marketplaces = append(c.Marketplaces, MarketplaceEntry{Name: name, Source: source})
	return nil
}

// RemoveMarketplace removes a marketplace from the config.
func (c *Config) RemoveMarketplace(name string) error {
	for i, m := range c.Marketplaces {
		if m.Name == name {
			c.Marketplaces = append(c.Marketplaces[:i], c.Marketplaces[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("marketplace %q not found", name)
}

// FindMarketplace returns a marketplace by name.
func (c *Config) FindMarketplace(name string) *MarketplaceEntry {
	for _, m := range c.Marketplaces {
		if m.Name == name {
			return &m
		}
	}
	return nil
}

package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/user/summon/internal/manifest"
)

// PluginJSON represents the plugin.json descriptor generated for each package.
type PluginJSON struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Version     string           `json:"version"`
	Author      *manifest.Author `json:"author,omitempty"`
	License     string           `json:"license,omitempty"`
}

// GeneratePluginJSON creates a .claude-plugin/plugin.json descriptor inside the
// given store package directory. The plugin.json is derived from the package's
// manifest and is used by Claude Code to identify and load the plugin.
func GeneratePluginJSON(storePackagePath string, m *manifest.Manifest) error {
	pluginDir := filepath.Join(storePackagePath, ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude-plugin dir: %w", err)
	}

	p := PluginJSON{
		Name:        m.Name,
		Description: m.Description,
		Version:     m.Version,
		Author:      m.Author,
		License:     m.License,
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plugin.json: %w", err)
	}

	return os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644)
}

// PluginJSONExists checks whether .claude-plugin/plugin.json already exists
// in the given package directory. Used to skip GeneratePluginJSON when the
// plugin ships its own descriptor.
func PluginJSONExists(storePackagePath string) bool {
	info, err := os.Stat(filepath.Join(storePackagePath, ".claude-plugin", "plugin.json"))
	return err == nil && !info.IsDir()
}

// Package marketplace generates marketplace.json and plugin.json descriptors
// that allow AI coding platforms (Claude, Copilot) to discover and load
// summon-managed packages.
package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/user/summon/internal/manifest"
	"github.com/user/summon/internal/registry"
)

// MarketplaceEntry represents a plugin entry in marketplace.json.
type MarketplaceEntry struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Version     string           `json:"version"`
	Source      string           `json:"source"`
	Author      *manifest.Author `json:"author,omitempty"`
	Homepage    string           `json:"homepage,omitempty"`
	Repository  string           `json:"repository,omitempty"`
}

// Marketplace represents the top-level marketplace.json structure.
type Marketplace struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Owner       *manifest.Author   `json:"owner,omitempty"`
	Plugins     []MarketplaceEntry `json:"plugins"`
}

// Generate creates a marketplace.json for a given platform by iterating over
// all packages in the registry, filtering by platform compatibility, and
// writing the result to platformDir/.claude-plugin/marketplace.json. For each
// compatible package a symlink is created at platformDir/plugins/<name>
// pointing back to the store so that Claude Code and VS Code Copilot can
// resolve the relative "./plugins/<name>" source paths.
func Generate(platformName, marketplaceName, storeDir, platformDir string, reg *registry.Registry) error {
	claudePluginDir := filepath.Join(platformDir, ".claude-plugin")
	if err := os.MkdirAll(claudePluginDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude-plugin dir: %w", err)
	}

	pluginsDir := filepath.Join(platformDir, "plugins")
	// Remove old plugins dir entirely so stale symlinks from uninstalled
	// packages are cleaned up. It is recreated below with only the current set.
	_ = os.RemoveAll(pluginsDir)
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("creating plugins dir: %w", err)
	}

	var plugins []MarketplaceEntry

	for name, entry := range reg.Packages {
		if !platformMatch(entry.Platforms, platformName) {
			continue
		}

		pkgPath := filepath.Join(storeDir, name)
		m, loadErr := manifest.Load(pkgPath)
		if loadErr != nil {
			m = &manifest.Manifest{
				Name:        name,
				Version:     entry.Version,
				Description: name,
			}
		}

		// Create symlink: platformDir/plugins/<name> → storeDir/<name>
		linkPath := filepath.Join(pluginsDir, name)
		_ = os.Remove(linkPath) // remove stale symlink if any
		if err := os.Symlink(pkgPath, linkPath); err != nil {
			return fmt.Errorf("creating plugin symlink for %s: %w", name, err)
		}

		me := MarketplaceEntry{
			Name:        m.Name,
			Description: m.Description,
			Version:     m.Version,
			Source:      "./plugins/" + name,
			Author:      m.Author,
			Homepage:    m.Homepage,
			Repository:  m.Repository,
		}
		plugins = append(plugins, me)
	}

	market := Marketplace{
		Name:        marketplaceName,
		Description: fmt.Sprintf("Summon %s package marketplace", marketplaceName),
		Owner:       &manifest.Author{Name: "summon"},
		Plugins:     plugins,
	}

	data, err := json.MarshalIndent(market, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling marketplace.json: %w", err)
	}

	return os.WriteFile(filepath.Join(claudePluginDir, "marketplace.json"), data, 0o644)
}

// platformMatch returns true if the package is compatible with the given platform.
// An empty platforms list means the package is compatible with all platforms.
func platformMatch(pkgPlatforms []string, platform string) bool {
	if len(pkgPlatforms) == 0 {
		return true
	}
	for _, p := range pkgPlatforms {
		if p == platform {
			return true
		}
	}
	return false
}

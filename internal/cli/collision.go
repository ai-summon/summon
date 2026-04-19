package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/skillscan"
)

// collisionCheckDeps holds injectable dependencies for collision detection.
type collisionCheckDeps struct {
	homeDir string // override for testing; empty = os.UserHomeDir()
}

// collisionResult holds collision findings for a single platform.
type collisionResult struct {
	CLI        string
	Collisions []skillscan.Collision
	Errors     []skillscan.ScanError
	PluginCount int
}

// collisionJSONEntry is the JSON representation of a collision.
type collisionJSONEntry struct {
	SkillName string                    `json:"skill_name"`
	Entries   []collisionJSONSkillEntry `json:"entries"`
}

// collisionJSONSkillEntry is the JSON representation of a skill entry in a collision.
type collisionJSONSkillEntry struct {
	PluginName  string `json:"plugin_name"`
	Marketplace string `json:"marketplace"`
	FilePath    string `json:"file_path"`
	Status      string `json:"status"` // "wins" or "shadowed"
}

// scanPlatformCollisions detects skill name collisions for a given platform adapter.
func scanPlatformCollisions(adapter platform.Adapter, scope platform.Scope, deps *collisionCheckDeps) *collisionResult {
	result := &collisionResult{CLI: adapter.Name()}

	homeDir := deps.homeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			result.Errors = append(result.Errors, skillscan.ScanError{
				PluginName: "(system)",
				Detail:     "cannot determine home directory",
				Err:        err,
			})
			return result
		}
	}

	var plugins []pluginScanInfo
	switch adapter.Name() {
	case "copilot":
		plugins = readCopilotPlugins(homeDir)
	case "claude":
		plugins = readClaudePlugins(homeDir)
	default:
		// Unsupported platform — try generic approach via adapter
		plugins = readPluginsViaAdapter(adapter, scope)
	}

	result.PluginCount = len(plugins)

	var allEntries []skillscan.SkillEntry
	for _, p := range plugins {
		entries, scanErrors := skillscan.ScanPlugin(p.Dir, p.Name, p.Marketplace, p.Order)
		allEntries = append(allEntries, entries...)
		result.Errors = append(result.Errors, scanErrors...)
	}

	result.Collisions = skillscan.DetectCollisions(allEntries)
	return result
}

// pluginScanInfo holds the info needed to scan a plugin for skills.
type pluginScanInfo struct {
	Name        string
	Marketplace string
	Dir         string
	Order       int
}

// readCopilotPlugins reads installed plugins from ~/.copilot/config.json.
// This provides authoritative install order and cache paths.
func readCopilotPlugins(homeDir string) []pluginScanInfo {
	configPath := filepath.Join(homeDir, ".copilot", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var cfg struct {
		InstalledPlugins []struct {
			Name        string `json:"name"`
			Marketplace string `json:"marketplace"`
			CachePath   string `json:"cache_path"`
			Enabled     *bool  `json:"enabled"`
		} `json:"installedPlugins"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	var plugins []pluginScanInfo
	for i, p := range cfg.InstalledPlugins {
		// Skip disabled plugins
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		if p.CachePath == "" {
			continue
		}
		if _, err := os.Stat(p.CachePath); err != nil {
			continue
		}
		plugins = append(plugins, pluginScanInfo{
			Name:        p.Name,
			Marketplace: p.Marketplace,
			Dir:         p.CachePath,
			Order:       i,
		})
	}
	return plugins
}

// readClaudePlugins reads installed plugins from Claude's metadata.
func readClaudePlugins(homeDir string) []pluginScanInfo {
	// Try installed_plugins.json
	metaPath := filepath.Join(homeDir, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil
	}

	var raw []struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Marketplace string `json:"marketplace"`
		Source      string `json:"source"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	var plugins []pluginScanInfo
	for i, p := range raw {
		name := p.Name
		if idx := strings.Index(name, "@"); idx > 0 {
			name = name[:idx]
		}
		dir := p.Path
		if dir == "" {
			continue
		}
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		marketplace := p.Marketplace
		if marketplace == "" {
			marketplace = p.Source
		}
		plugins = append(plugins, pluginScanInfo{
			Name:        name,
			Marketplace: marketplace,
			Dir:         dir,
			Order:       i,
		})
	}
	return plugins
}

// readPluginsViaAdapter is a generic fallback that uses the adapter interface.
// Order may not be authoritative.
func readPluginsViaAdapter(adapter platform.Adapter, scope platform.Scope) []pluginScanInfo {
	installed, err := adapter.ListInstalled(scope)
	if err != nil {
		return nil
	}

	var plugins []pluginScanInfo
	for i, p := range installed {
		dir, err := adapter.FindPluginDir(p.Name, scope)
		if err != nil {
			continue
		}
		plugins = append(plugins, pluginScanInfo{
			Name:        p.Name,
			Marketplace: p.Source,
			Dir:         dir,
			Order:       i,
		})
	}
	return plugins
}

// printCollisions prints collision warnings to the given writer using styled output.
func printCollisions(w io.Writer, result *collisionResult, s Styles) {
	if len(result.Collisions) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "\n%s Skill name collisions detected:\n\n", s.StatusIcon("warn"))

	for _, c := range result.Collisions {
		_, _ = fmt.Fprintf(w, "  Skill %s:\n", s.Name.Render(fmt.Sprintf("%q", c.SkillName)))

		for i, e := range c.Entries {
			var statusLine string
			marketplace := ""
			if e.Marketplace != "" {
				marketplace = fmt.Sprintf(" %s", s.Dim.Render(fmt.Sprintf("(%s)", e.Marketplace)))
			}

			if i == 0 {
				statusLine = fmt.Sprintf("    %s %s%s — %s",
					s.StatusIcon("pass"),
					e.PluginName,
					marketplace,
					s.Dim.Render("WINS (loaded first)"),
				)
			} else {
				statusLine = fmt.Sprintf("    %s %s%s — %s",
					s.StatusIcon("fail"),
					e.PluginName,
					marketplace,
					s.Warn.Render("SHADOWED"),
				)
			}
			_, _ = fmt.Fprintln(w, statusLine)
			_, _ = fmt.Fprintf(w, "      %s %s\n", s.Dim.Render("└─"), s.Dim.Render(e.FilePath))
		}
		_, _ = fmt.Fprintln(w)
	}

	noun := "collision"
	if len(result.Collisions) != 1 {
		noun = "collisions"
	}
	_, _ = fmt.Fprintf(w, "  %s %s across %d installed plugins.\n",
		s.Dim.Render(fmt.Sprintf("%d %s found", len(result.Collisions), noun)),
		"",
		result.PluginCount,
	)
	_, _ = fmt.Fprintf(w, "  %s\n", s.Dim.Render("Shadowed skills will not be available as slash commands."))
}

// collisionsToJSON converts collision results to a JSON-serializable structure.
func collisionsToJSON(collisions []skillscan.Collision) []collisionJSONEntry {
	entries := make([]collisionJSONEntry, len(collisions))
	for i, c := range collisions {
		jEntries := make([]collisionJSONSkillEntry, len(c.Entries))
		for j, e := range c.Entries {
			status := "shadowed"
			if j == 0 {
				status = "wins"
			}
			jEntries[j] = collisionJSONSkillEntry{
				PluginName:  e.PluginName,
				Marketplace: e.Marketplace,
				FilePath:    e.FilePath,
				Status:      status,
			}
		}
		entries[i] = collisionJSONEntry{
			SkillName: c.SkillName,
			Entries:   jEntries,
		}
	}
	return entries
}

// printInstallCollisionWarning prints a focused collision warning after install,
// only showing collisions that involve the newly installed packages.
func printInstallCollisionWarning(w io.Writer, result *collisionResult, newPackages []string, s Styles) {
	if len(result.Collisions) == 0 {
		return
	}

	newSet := make(map[string]bool, len(newPackages))
	for _, p := range newPackages {
		newSet[p] = true
	}

	var relevant []skillscan.Collision
	for _, c := range result.Collisions {
		involves := false
		for _, e := range c.Entries {
			if newSet[e.PluginName] {
				involves = true
				break
			}
		}
		if involves {
			relevant = append(relevant, c)
		}
	}

	if len(relevant) == 0 {
		return
	}

	filtered := &collisionResult{
		CLI:         result.CLI,
		Collisions:  relevant,
		PluginCount: result.PluginCount,
	}
	printCollisions(w, filtered, s)
}

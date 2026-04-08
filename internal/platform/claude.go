package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeAdapter implements the Adapter interface for Claude Code.
// It manages marketplace registrations in Claude's settings.json by writing
// entries to the extraKnownMarketplaces map.
type ClaudeAdapter struct {
	ProjectDir string
	// HomeDir overrides the user home directory for settings path resolution.
	// When empty, os.UserHomeDir() is used. This is primarily for testing.
	HomeDir string
}

func (c *ClaudeAdapter) Name() string {
	return "claude"
}

func (c *ClaudeAdapter) Detect() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".claude"))
	return err == nil
}

func (c *ClaudeAdapter) SettingsPath(scope Scope) string {
	if scope == ScopeGlobal {
		home := c.HomeDir
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		return filepath.Join(home, ".claude", "settings.json")
	}
	if scope == ScopeLocal {
		return filepath.Join(c.ProjectDir, ".claude", "settings.local.json")
	}
	// ScopeProject: shared project settings
	return filepath.Join(c.ProjectDir, ".claude", "settings.json")
}

func (c *ClaudeAdapter) Register(marketplacePath string, marketplaceName string, scope Scope) error {
	settingsPath := c.SettingsPath(scope)
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		return err
	}

	ekm, ok := settings["extraKnownMarketplaces"].(map[string]interface{})
	if !ok {
		ekm = make(map[string]interface{})
	}

	absPath, err := filepath.Abs(marketplacePath)
	if err != nil {
		absPath = marketplacePath
	}

	ekm[marketplaceName] = map[string]interface{}{
		"source": map[string]interface{}{
			"source": "directory",
			"path":   absPath,
		},
	}

	settings["extraKnownMarketplaces"] = ekm
	return writeJSONFile(settingsPath, settings)
}

func (c *ClaudeAdapter) Unregister(marketplaceName string, scope Scope) error {
	settingsPath := c.SettingsPath(scope)
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		// Best-effort cleanup: if the file can't be parsed, there's nothing
		// safe to remove. Silently skip rather than block uninstall.
		return nil
	}

	ekm, ok := settings["extraKnownMarketplaces"].(map[string]interface{})
	if !ok {
		return nil
	}

	delete(ekm, marketplaceName)
	if len(ekm) == 0 {
		delete(settings, "extraKnownMarketplaces")
	} else {
		settings["extraKnownMarketplaces"] = ekm
	}

	return writeJSONFile(settingsPath, settings)
}

func (c *ClaudeAdapter) EnablePlugin(pluginName string, marketplaceName string, storeDir string, scope Scope) error {
	settingsPath := c.SettingsPath(scope)
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		return err
	}

	ep, ok := settings["enabledPlugins"].(map[string]interface{})
	if !ok {
		ep = make(map[string]interface{})
	}

	key := pluginName + "@" + marketplaceName
	ep[key] = true
	settings["enabledPlugins"] = ep

	return writeJSONFile(settingsPath, settings)
}

func (c *ClaudeAdapter) DisablePlugin(pluginName string, marketplaceName string, storeDir string, scope Scope) error {
	settingsPath := c.SettingsPath(scope)
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		// Best-effort cleanup: if the file can't be parsed, there's nothing
		// safe to remove. Silently skip rather than block uninstall.
		return nil
	}

	ep, ok := settings["enabledPlugins"].(map[string]interface{})
	if !ok {
		return nil
	}

	key := pluginName + "@" + marketplaceName
	delete(ep, key)
	if len(ep) == 0 {
		delete(settings, "enabledPlugins")
	}

	return writeJSONFile(settingsPath, settings)
}

// readJSONFile reads and unmarshals a JSON file into a generic map.
// Returns an empty map if the file does not exist or is empty/whitespace-only.
// Returns an error if the file exists but is not valid JSON.
func readJSONFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("cannot safely read settings from %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return make(map[string]interface{}), nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("cannot safely read settings from %s: file exists but is not valid JSON (%w); fix the file manually or remove non-JSON content (e.g., comments, trailing commas)", path, err)
	}
	return result, nil
}

// writeJSONFile marshals a map to pretty-printed JSON and atomically writes it
// to the given path using temp-file-then-rename, creating parent directories
// as needed.
func writeJSONFile(path string, data map[string]interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	tmp, err := os.CreateTemp(dir, ".summon-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file for settings: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp settings file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp settings file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions on temp settings file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp settings file: %w", err)
	}
	return nil
}

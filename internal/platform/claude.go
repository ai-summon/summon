package platform

import (
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
		home, _ := os.UserHomeDir()
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
		settings = make(map[string]interface{})
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
		settings = make(map[string]interface{})
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
// Returns an error if the file cannot be read or is not valid JSON.
func readJSONFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// writeJSONFile marshals a map to pretty-printed JSON and writes it to the
// given path, creating parent directories as needed.
func writeJSONFile(path string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

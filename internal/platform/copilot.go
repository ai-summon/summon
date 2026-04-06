package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CopilotAdapter implements the Adapter interface for VS Code Copilot.
// It supports both local (.vscode/settings.json) and global (VS Code user
// settings) scope for marketplace registration.
//
// VS Code has application-scoped and workspace-scoped settings. The
// chat.plugins.marketplaces and chat.pluginLocations settings are
// application-scoped — VS Code only reads them from user-level settings.
// For local scope installs, the adapter writes these keys to BOTH the
// workspace settings (for extraKnownMarketplaces/enabledPlugins workspace
// recommendations) AND user-level settings (for actual plugin activation).
type CopilotAdapter struct {
	ProjectDir string
	// GlobalSettingsDir overrides the VS Code user settings directory.
	// When empty, the platform default is used. This is primarily for testing.
	GlobalSettingsDir string
}

func (v *CopilotAdapter) Name() string {
	return "copilot"
}

func (v *CopilotAdapter) Detect() bool {
	_, err := os.Stat(v.detectDir())
	return err == nil
}

// detectDir returns the real VS Code user config directory for this platform.
// It ignores GlobalSettingsDir so that detection reflects whether VS Code is
// actually installed, not where tests redirect settings writes.
func (v *CopilotAdapter) detectDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User")
	case "linux":
		return filepath.Join(home, ".config", "Code", "User")
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "Code", "User")
	default:
		return filepath.Join(home, ".config", "Code", "User")
	}
}

func (v *CopilotAdapter) SettingsPath(scope Scope) string {
	if scope == ScopeGlobal {
		return v.globalSettingsPath()
	}
	return filepath.Join(v.ProjectDir, ".vscode", "settings.json")
}

func (v *CopilotAdapter) globalSettingsPath() string {
	if v.GlobalSettingsDir != "" {
		return filepath.Join(v.GlobalSettingsDir, "settings.json")
	}
	// SUMMON_VSCODE_SETTINGS_DIR allows e2e tests (which invoke the binary as
	// a subprocess) to redirect user-level settings writes to a temp directory.
	if dir := os.Getenv("SUMMON_VSCODE_SETTINGS_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json")
	case "linux":
		return filepath.Join(home, ".config", "Code", "User", "settings.json")
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "Code", "User", "settings.json")
	default:
		return filepath.Join(home, ".config", "Code", "User", "settings.json")
	}
}

func (v *CopilotAdapter) Register(marketplacePath string, marketplaceName string, scope Scope) error {
	absPath, err := filepath.Abs(marketplacePath)
	if err != nil {
		absPath = marketplacePath
	}
	uri := "file://" + absPath

	if scope == ScopeProject || scope == ScopeLocal {
		wsPath := v.SettingsPath(scope)
		// Write chat.plugins.marketplaces to workspace settings.
		if err := v.addMarketplaceURI(wsPath, uri); err != nil {
			return fmt.Errorf("writing workspace settings: %w", err)
		}
		// Write extraKnownMarketplaces for workspace-level recommendations.
		if err := v.addExtraKnownMarketplace(wsPath, marketplaceName, absPath); err != nil {
			return fmt.Errorf("writing workspace extraKnownMarketplaces: %w", err)
		}
		// For user scope (ScopeLocal), also propagate to user-level settings so
		// VS Code actually activates the marketplace.
		if scope == ScopeLocal {
			appSettingsPath := v.globalSettingsPath()
			if err := v.ensurePluginsEnabled(appSettingsPath); err != nil {
				return fmt.Errorf("enabling chat.plugins.enabled: %w", err)
			}
			if err := v.addMarketplaceURI(appSettingsPath, uri); err != nil {
				return fmt.Errorf("writing user-level settings: %w", err)
			}
		}
		return nil
	}

	// ScopeUser / ScopeGlobal: write only to user-level settings.
	appSettingsPath := v.globalSettingsPath()
	if err := v.ensurePluginsEnabled(appSettingsPath); err != nil {
		return fmt.Errorf("enabling chat.plugins.enabled: %w", err)
	}
	if err := v.addMarketplaceURI(appSettingsPath, uri); err != nil {
		return fmt.Errorf("writing user-level settings: %w", err)
	}
	return nil
}

func (v *CopilotAdapter) Unregister(marketplaceName string, scope Scope) error {
	if scope == ScopeProject || scope == ScopeLocal {
		wsPath := v.SettingsPath(scope)
		_ = v.removeMarketplaceURI(wsPath, marketplaceName)
		_ = v.removeExtraKnownMarketplace(wsPath, marketplaceName)
		if scope == ScopeLocal {
			// Also clean user-level settings for local scope.
			appSettingsPath := v.globalSettingsPath()
			_ = v.removeMarketplaceURI(appSettingsPath, marketplaceName)
		}
		return nil
	}

	// ScopeUser / ScopeGlobal.
	appSettingsPath := v.globalSettingsPath()
	return v.removeMarketplaceURI(appSettingsPath, marketplaceName)
}

// EnablePlugin registers the plugin directory via chat.pluginLocations
// and (for local/project workspace scopes) enabledPlugins.
func (v *CopilotAdapter) EnablePlugin(pluginName string, marketplaceName string, storeDir string, scope Scope) error {
	pluginPath, err := v.resolvePluginPath(storeDir, pluginName)
	if err != nil {
		return err
	}

	if scope == ScopeProject || scope == ScopeLocal {
		wsPath := v.SettingsPath(scope)
		// Write chat.pluginLocations to workspace settings.
		if err := v.addPluginLocation(wsPath, pluginPath); err != nil {
			return fmt.Errorf("writing workspace settings: %w", err)
		}
		// enabledPlugins is workspace-scoped.
		if err := v.addEnabledPlugin(wsPath, pluginName, marketplaceName); err != nil {
			return fmt.Errorf("writing workspace enabledPlugins: %w", err)
		}
		// For local scope, also propagate to user-level settings for VS Code activation.
		if scope == ScopeLocal {
			appSettingsPath := v.globalSettingsPath()
			if err := v.addPluginLocation(appSettingsPath, pluginPath); err != nil {
				return fmt.Errorf("writing user-level settings: %w", err)
			}
		}
		return nil
	}

	// ScopeUser / ScopeGlobal: user-level only.
	appSettingsPath := v.globalSettingsPath()
	if err := v.addPluginLocation(appSettingsPath, pluginPath); err != nil {
		return fmt.Errorf("writing user-level settings: %w", err)
	}
	return nil
}

// DisablePlugin removes the plugin from chat.pluginLocations and
// (for workspace scopes) from enabledPlugins.
func (v *CopilotAdapter) DisablePlugin(pluginName string, marketplaceName string, storeDir string, scope Scope) error {
	pluginPath, err := v.resolvePluginPath(storeDir, pluginName)
	if err != nil {
		// Plugin directory likely already removed; nothing to clean up.
		return nil
	}

	if scope == ScopeProject || scope == ScopeLocal {
		wsPath := v.SettingsPath(scope)
		_ = v.removePluginLocation(wsPath, pluginPath)
		_ = v.removeEnabledPlugin(wsPath, pluginName, marketplaceName)
		if scope == ScopeLocal {
			// Also clean user-level settings for local scope.
			appSettingsPath := v.globalSettingsPath()
			_ = v.removePluginLocation(appSettingsPath, pluginPath)
		}
		return nil
	}

	// ScopeUser / ScopeGlobal.
	appSettingsPath := v.globalSettingsPath()
	_ = v.removePluginLocation(appSettingsPath, pluginPath)
	return nil
}

// --- helpers ---

func (v *CopilotAdapter) resolvePluginPath(storeDir, pluginName string) (string, error) {
	pluginPath := filepath.Join(storeDir, pluginName)
	pluginPath, err := filepath.Abs(pluginPath)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(pluginPath)); err == nil {
		pluginPath = filepath.Join(resolved, filepath.Base(pluginPath))
	}
	return pluginPath, nil
}

func (v *CopilotAdapter) addMarketplaceURI(settingsPath, uri string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	}
	var marketplaces []interface{}
	if existing, ok := settings["chat.plugins.marketplaces"].([]interface{}); ok {
		marketplaces = existing
	}
	for _, m := range marketplaces {
		if m == uri {
			return nil
		}
	}
	marketplaces = append(marketplaces, uri)
	settings["chat.plugins.marketplaces"] = marketplaces
	return writeJSONFile(settingsPath, settings)
}

func (v *CopilotAdapter) removeMarketplaceURI(settingsPath, marketplaceName string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		return nil
	}
	if arr, ok := settings["chat.plugins.marketplaces"].([]interface{}); ok {
		var filtered []interface{}
		for _, m := range arr {
			s, _ := m.(string)
			if s != "" && !containsSegment(s, marketplaceName) {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) == 0 {
			delete(settings, "chat.plugins.marketplaces")
		} else {
			settings["chat.plugins.marketplaces"] = filtered
		}
	}
	return writeJSONFile(settingsPath, settings)
}

func (v *CopilotAdapter) addExtraKnownMarketplace(settingsPath, marketplaceName, absPath string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	}
	ekm, ok := settings["extraKnownMarketplaces"].(map[string]interface{})
	if !ok {
		ekm = make(map[string]interface{})
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

func (v *CopilotAdapter) removeExtraKnownMarketplace(settingsPath, marketplaceName string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		return nil
	}
	if ekm, ok := settings["extraKnownMarketplaces"].(map[string]interface{}); ok {
		delete(ekm, marketplaceName)
		if len(ekm) == 0 {
			delete(settings, "extraKnownMarketplaces")
		} else {
			settings["extraKnownMarketplaces"] = ekm
		}
	}
	return writeJSONFile(settingsPath, settings)
}

func (v *CopilotAdapter) addPluginLocation(settingsPath, pluginPath string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	}
	pl, ok := settings["chat.pluginLocations"].(map[string]interface{})
	if !ok {
		pl = make(map[string]interface{})
	}
	pl[pluginPath] = true
	settings["chat.pluginLocations"] = pl
	return writeJSONFile(settingsPath, settings)
}

func (v *CopilotAdapter) removePluginLocation(settingsPath, pluginPath string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		return nil
	}
	pl, ok := settings["chat.pluginLocations"].(map[string]interface{})
	if !ok {
		return nil
	}
	delete(pl, pluginPath)
	if len(pl) == 0 {
		delete(settings, "chat.pluginLocations")
	} else {
		settings["chat.pluginLocations"] = pl
	}
	return writeJSONFile(settingsPath, settings)
}

func (v *CopilotAdapter) addEnabledPlugin(settingsPath, pluginName, marketplaceName string) error {
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

func (v *CopilotAdapter) removeEnabledPlugin(settingsPath, pluginName, marketplaceName string) error {
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
	} else {
		settings["enabledPlugins"] = ep
	}
	return writeJSONFile(settingsPath, settings)
}

// containsSegment checks if a URI string contains a path segment matching name.
// It handles both forward slashes (Unix/URLs) and backslashes (Windows paths).
func containsSegment(uri, name string) bool {
	// Normalise to forward slashes so matching works on Windows paths too.
	norm := strings.ReplaceAll(uri, `\`, "/")
	return strings.Contains(norm, "/"+name+"/") || strings.HasSuffix(norm, "/"+name)
}

// ensurePluginsEnabled sets chat.plugins.enabled to true in the given VS Code
// settings file. This is the master toggle for the agent plugins preview
// feature — without it VS Code ignores all other plugin configuration.
func (v *CopilotAdapter) ensurePluginsEnabled(settingsPath string) error {
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	}
	if enabled, ok := settings["chat.plugins.enabled"].(bool); ok && enabled {
		return nil
	}
	settings["chat.plugins.enabled"] = true
	return writeJSONFile(settingsPath, settings)
}

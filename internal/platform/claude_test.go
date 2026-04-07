package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeAdapter_Name(t *testing.T) {
	a := &ClaudeAdapter{ProjectDir: "/tmp/proj"}
	assert.Equal(t, "claude", a.Name())
}

func TestClaudeAdapter_SettingsPath_Local(t *testing.T) {
	a := &ClaudeAdapter{ProjectDir: "/my/project"}
	path := a.SettingsPath(ScopeLocal)
	assert.Equal(t, filepath.Join("/my/project", ".claude", "settings.local.json"), path)
}

func TestClaudeAdapter_SettingsPath_Project(t *testing.T) {
	a := &ClaudeAdapter{ProjectDir: "/my/project"}
	path := a.SettingsPath(ScopeProject)
	assert.Equal(t, filepath.Join("/my/project", ".claude", "settings.json"), path)
}

func TestClaudeAdapter_SettingsPath_Global(t *testing.T) {
	a := &ClaudeAdapter{ProjectDir: "/my/project"}
	path := a.SettingsPath(ScopeGlobal)
	assert.True(t, strings.HasSuffix(path, filepath.Join(".claude", "settings.json")))
	// Global path should NOT use the project dir
	assert.False(t, strings.HasPrefix(path, "/my/project"))
}

func TestClaudeAdapter_Register(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	err := a.Register("/store/claude", "test-mkt", ScopeLocal)
	require.NoError(t, err)

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	ekm, ok := settings["extraKnownMarketplaces"].(map[string]interface{})
	require.True(t, ok, "extraKnownMarketplaces should be present")

	entry, ok := ekm["test-mkt"]
	require.True(t, ok, "test-mkt entry should be present")

	entryMap := entry.(map[string]interface{})
	source := entryMap["source"].(map[string]interface{})
	assert.Equal(t, "directory", source["source"])
	assert.NotEmpty(t, source["path"])
}

func TestClaudeAdapter_Register_ProjectScope(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	err := a.Register("/store/claude", "project-mkt", ScopeProject)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))
	_, ok := settings["extraKnownMarketplaces"].(map[string]interface{})["project-mkt"]
	assert.True(t, ok)
}

func TestClaudeAdapter_Register_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	require.NoError(t, a.Register("/store/claude", "mkt", ScopeProject))
	require.NoError(t, a.Register("/store/claude", "mkt", ScopeProject))

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	ekm := settings["extraKnownMarketplaces"].(map[string]interface{})
	// Only one entry, not duplicated
	assert.Len(t, ekm, 1)
}

func TestClaudeAdapter_Unregister(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	require.NoError(t, a.Register("/store/claude", "mkt-to-remove", ScopeProject))
	require.NoError(t, a.Unregister("mkt-to-remove", ScopeProject))

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	// extraKnownMarketplaces should be removed entirely when empty
	_, hasEKM := settings["extraKnownMarketplaces"]
	assert.False(t, hasEKM, "extraKnownMarketplaces should be removed when empty")
}

func TestClaudeAdapter_Unregister_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	// Unregister when no settings file exists should not error
	err := a.Unregister("nonexistent", ScopeProject)
	assert.NoError(t, err)
}

func TestClaudeAdapter_EnablePlugin(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	err := a.EnablePlugin("my-plugin", "summon-local", "", ScopeProject)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	ep, ok := settings["enabledPlugins"].(map[string]interface{})
	require.True(t, ok, "enabledPlugins should be present")
	assert.Equal(t, true, ep["my-plugin@summon-local"])
}

func TestClaudeAdapter_EnablePlugin_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	require.NoError(t, a.EnablePlugin("p", "mkt", "", ScopeProject))
	require.NoError(t, a.EnablePlugin("p", "mkt", "", ScopeProject))

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	ep := settings["enabledPlugins"].(map[string]interface{})
	assert.Len(t, ep, 1)
	assert.Equal(t, true, ep["p@mkt"])
}

func TestClaudeAdapter_EnablePlugin_PreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	// Register a marketplace first, then enable a plugin
	require.NoError(t, a.Register("/store", "mkt", ScopeProject))
	require.NoError(t, a.EnablePlugin("p1", "mkt", "", ScopeProject))

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	// Both keys should coexist
	_, hasEKM := settings["extraKnownMarketplaces"]
	assert.True(t, hasEKM, "extraKnownMarketplaces should still be present")
	_, hasEP := settings["enabledPlugins"]
	assert.True(t, hasEP, "enabledPlugins should be present")
}

func TestClaudeAdapter_DisablePlugin(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	require.NoError(t, a.EnablePlugin("p1", "mkt", "", ScopeProject))
	require.NoError(t, a.EnablePlugin("p2", "mkt", "", ScopeProject))
	require.NoError(t, a.DisablePlugin("p1", "mkt", "", ScopeProject))

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	ep := settings["enabledPlugins"].(map[string]interface{})
	assert.Len(t, ep, 1)
	_, hasP1 := ep["p1@mkt"]
	assert.False(t, hasP1, "p1@mkt should be removed")
	assert.Equal(t, true, ep["p2@mkt"])
}

func TestClaudeAdapter_DisablePlugin_RemovesKeyWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	require.NoError(t, a.EnablePlugin("p", "mkt", "", ScopeProject))
	require.NoError(t, a.DisablePlugin("p", "mkt", "", ScopeProject))

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	_, hasEP := settings["enabledPlugins"]
	assert.False(t, hasEP, "enabledPlugins should be removed when empty")
}

func TestClaudeAdapter_DisablePlugin_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	err := a.DisablePlugin("nonexistent", "mkt", "", ScopeProject)
	assert.NoError(t, err)
}

func TestClaudeAdapter_Local_UsesSettingsLocalJSON(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	// Operations with ScopeLocal should go to settings.local.json
	require.NoError(t, a.Register("/store/claude", "local-mkt", ScopeLocal))

	// settings.local.json should exist
	localPath := filepath.Join(tmpDir, ".claude", "settings.local.json")
	data, err := os.ReadFile(localPath)
	require.NoError(t, err)
	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))
	ekm, ok := settings["extraKnownMarketplaces"].(map[string]interface{})
	require.True(t, ok)
	_, hasLocalMkt := ekm["local-mkt"]
	assert.True(t, hasLocalMkt, "local-mkt should be in settings.local.json")

	// settings.json (project scope) should NOT be written
	_, err = os.Stat(filepath.Join(tmpDir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(err), "settings.json should not be created for ScopeLocal ops")
}

package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

// --- readJSONFile tests ---

func TestReadJSONFile_NotFound(t *testing.T) {
	result, err := readJSONFile(filepath.Join(t.TempDir(), "nonexistent.json"))
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestReadJSONFile_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o644))

	result, err := readJSONFile(path)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestReadJSONFile_WhitespaceOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ws.json")
	require.NoError(t, os.WriteFile(path, []byte("  \n\t\n  "), 0o644))

	result, err := readJSONFile(path)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestReadJSONFile_ValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"key": "value", "num": 42}`), 0o644))

	result, err := readJSONFile(path)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
	assert.Equal(t, float64(42), result["num"])
}

func TestReadJSONFile_InvalidJSON_Comments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jsonc.json")
	content := `{
  // This is a comment
  "key": "value"
}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	result, err := readJSONFile(path)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestReadJSONFile_InvalidJSON_TrailingComma(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trailing.json")
	content := `{"key": "value",}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	result, err := readJSONFile(path)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestReadJSONFile_InvalidJSON_Garbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.json")
	require.NoError(t, os.WriteFile(path, []byte("not json at all"), 0o644))

	result, err := readJSONFile(path)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

func TestReadJSONFile_Unreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "unreadable.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"key":"val"}`), 0o644))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	result, err := readJSONFile(path)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

// --- writeJSONFile tests ---

func TestWriteJSONFile_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	data := map[string]interface{}{"hello": "world"}
	require.NoError(t, writeJSONFile(path, data))

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// Should be pretty-printed with 2-space indent and trailing newline
	assert.Contains(t, string(content), "  \"hello\": \"world\"")
	assert.True(t, strings.HasSuffix(string(content), "\n"))

	// Should be parseable
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.Equal(t, "world", parsed["hello"])

	// Check permissions (Windows maps 0644 to 0666, skip there)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
}

func TestWriteJSONFile_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "dir", "settings.json")
	data := map[string]interface{}{"created": true}
	require.NoError(t, writeJSONFile(path, data))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.Equal(t, true, parsed["created"])
}

func TestWriteJSONFile_NoTempFilesRemain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	require.NoError(t, writeJSONFile(path, map[string]interface{}{"key": "val"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.Contains(e.Name(), ".summon-settings-"),
			"temp file should be cleaned up: %s", e.Name())
	}
}

func TestWriteJSONFile_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Write initial content
	require.NoError(t, writeJSONFile(path, map[string]interface{}{"version": float64(1)}))

	// Overwrite atomically
	require.NoError(t, writeJSONFile(path, map[string]interface{}{"version": float64(2)}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.Equal(t, float64(2), parsed["version"])
}

// --- T013: Parse-failure preservation test for Claude adapter ---

func TestClaudeAdapter_Register_ParseFailurePreservesFile(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	// Create settings.json with JSONC content
	settingsDir := filepath.Join(tmpDir, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0o755))
	settingsPath := filepath.Join(settingsDir, "settings.json")
	jsoncContent := []byte(`{
  // User comment
  "editor.fontSize": 14,
  "theme": "dark"
}`)
	require.NoError(t, os.WriteFile(settingsPath, jsoncContent, 0o644))

	err := a.Register("/store/claude", "test-mkt", ScopeProject)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")

	// File must be byte-identical
	after, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, jsoncContent, after, "file should be byte-identical after failed parse")
}

func TestClaudeAdapter_EnablePlugin_ParseFailurePreservesFile(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	settingsDir := filepath.Join(tmpDir, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0o755))
	settingsPath := filepath.Join(settingsDir, "settings.json")
	jsoncContent := []byte(`{
  // User comment
  "editor.fontSize": 14
}`)
	require.NoError(t, os.WriteFile(settingsPath, jsoncContent, 0o644))

	err := a.EnablePlugin("my-plugin", "summon-local", "", ScopeProject)
	require.Error(t, err)

	after, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, jsoncContent, after)
}

// --- T015: Non-destructive merge regression test for Claude adapter ---

func TestClaudeAdapter_NonDestructiveMerge(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	// Create settings with many unrelated keys
	settingsDir := filepath.Join(tmpDir, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0o755))
	settingsPath := filepath.Join(settingsDir, "settings.json")

	original := map[string]interface{}{
		"editor.fontSize":        float64(14),
		"editor.tabSize":         float64(4),
		"editor.wordWrap":        "on",
		"workbench.colorTheme":   "Monokai",
		"terminal.integrated":    true,
		"files.autoSave":         "afterDelay",
		"files.autoSaveDelay":    float64(1000),
		"breadcrumbs.enabled":    true,
		"editor.minimap.enabled": false,
		"window.zoomLevel":       float64(0),
		"git.autofetch":          true,
		"debug.console.fontSize": float64(12),
	}
	require.NoError(t, writeJSONFile(settingsPath, original))

	// Full install cycle
	require.NoError(t, a.Register("/store", "mkt", ScopeProject))
	require.NoError(t, a.EnablePlugin("p1", "mkt", "", ScopeProject))
	require.NoError(t, a.DisablePlugin("p1", "mkt", "", ScopeProject))
	require.NoError(t, a.Unregister("mkt", ScopeProject))

	// Read back and verify all original keys survive
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result))

	for key, expected := range original {
		assert.Equal(t, expected, result[key], "key %q should be preserved", key)
	}
}

// --- T017: Atomic write integration test for Claude adapter ---

func TestClaudeAdapter_Register_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	require.NoError(t, a.Register("/store/claude", "test-mkt", ScopeProject))

	settingsDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Output should be valid JSON
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	// No temp files should remain
	entries, err := os.ReadDir(settingsDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.Contains(e.Name(), ".summon-settings-"),
			"temp file should be cleaned up: %s", e.Name())
	}
}

// --- T019: Write-failure preservation test ---

func TestWriteJSONFile_FailurePreservesOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permissions not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	originalContent := []byte(`{"preserved": true}` + "\n")
	require.NoError(t, os.WriteFile(path, originalContent, 0o644))

	// Attempt to write to a path inside a read-only directory
	roDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(roDir, 0o755))
	roPath := filepath.Join(roDir, "settings.json")
	require.NoError(t, os.WriteFile(roPath, originalContent, 0o644))
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	err := writeJSONFile(roPath, map[string]interface{}{"new": "data"})
	require.Error(t, err, "write to read-only dir should fail")

	// Original file should be unchanged
	after, err := os.ReadFile(roPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, after, "original file should be preserved after write failure")
}

// --- T020: Scope-specific parse-failure tests for Claude adapter ---

func TestClaudeAdapter_ParseFailure_AllScopes(t *testing.T) {
	jsoncContent := []byte(`{
  // comment
  "key": "value"
}`)

	tests := []struct {
		name  string
		scope Scope
		setup func(tmpDir string) string // returns settings path
	}{
		{
			name:  "ScopeLocal",
			scope: ScopeLocal,
			setup: func(tmpDir string) string {
				dir := filepath.Join(tmpDir, ".claude")
				os.MkdirAll(dir, 0o755)
				p := filepath.Join(dir, "settings.local.json")
				os.WriteFile(p, jsoncContent, 0o644)
				return p
			},
		},
		{
			name:  "ScopeProject",
			scope: ScopeProject,
			setup: func(tmpDir string) string {
				dir := filepath.Join(tmpDir, ".claude")
				os.MkdirAll(dir, 0o755)
				p := filepath.Join(dir, "settings.json")
				os.WriteFile(p, jsoncContent, 0o644)
				return p
			},
		},
		{
			name:  "ScopeGlobal",
			scope: ScopeGlobal,
			setup: func(tmpDir string) string {
				dir := filepath.Join(tmpDir, ".claude")
				os.MkdirAll(dir, 0o755)
				p := filepath.Join(dir, "settings.json")
				os.WriteFile(p, jsoncContent, 0o644)
				return p
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			a := &ClaudeAdapter{ProjectDir: tmpDir, HomeDir: tmpDir}
			settingsPath := tc.setup(tmpDir)

			err := a.Register("/store", "mkt", tc.scope)
			require.Error(t, err, "should refuse to write on parse failure")
			assert.Contains(t, err.Error(), "not valid JSON")

			after, err := os.ReadFile(settingsPath)
			require.NoError(t, err)
			assert.Equal(t, jsoncContent, after, "file should be unchanged")
		})
	}
}

// --- T022: Missing-file creation test for Claude adapter ---

func TestClaudeAdapter_MissingFile_CreatesWithRequiredKeys(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	// File does not exist — Register should create it
	err := a.Register("/store", "mkt", ScopeProject)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	require.NoError(t, err)
	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))

	// Should only have extraKnownMarketplaces
	_, hasEKM := settings["extraKnownMarketplaces"]
	assert.True(t, hasEKM, "should have extraKnownMarketplaces")
	assert.Len(t, settings, 1, "should only have the registered key")
}

// --- T023: Error message format test ---

func TestClaudeAdapter_Register_ErrorMessageFormat(t *testing.T) {
	tmpDir := t.TempDir()
	a := &ClaudeAdapter{ProjectDir: tmpDir}

	settingsDir := filepath.Join(tmpDir, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0o755))
	settingsPath := filepath.Join(settingsDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{// comment}`), 0o644))

	err := a.Register("/store", "mkt", ScopeProject)
	require.Error(t, err)
	errMsg := err.Error()
	assert.Contains(t, errMsg, settingsPath)
	assert.Contains(t, errMsg, "not valid JSON")
	assert.Contains(t, errMsg, "comments")
	assert.Contains(t, errMsg, "trailing commas")
}

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

// canonicalPluginPath returns the expected path after symlink resolution,
// matching what EnablePlugin/DisablePlugin store in settings.
func canonicalPluginPath(storeDir, name string) string {
	p, _ := filepath.Abs(filepath.Join(storeDir, name))
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(resolved, name)
	}
	return p
}

// newTestCopilotAdapter creates an adapter with both project and global
// settings directories pointing to temp subdirectories.
func newTestCopilotAdapter(t *testing.T) (*CopilotAdapter, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global")
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	a := &CopilotAdapter{
		ProjectDir:        tmpDir,
		GlobalSettingsDir: globalDir,
	}
	return a, tmpDir, globalDir
}

func readSettings(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var s map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &s))
	return s
}

func TestCopilotAdapter_Name(t *testing.T) {
	a := &CopilotAdapter{ProjectDir: "/some/project"}
	assert.Equal(t, "copilot", a.Name())
}

func TestCopilotAdapter_SettingsPath_Local(t *testing.T) {
	a := &CopilotAdapter{ProjectDir: "/my/project"}
	path := a.SettingsPath(ScopeLocal)
	assert.Equal(t, filepath.Join("/my/project", ".vscode", "settings.json"), path)
}

func TestCopilotAdapter_Register_Local(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)

	err := a.Register("/store/copilot", "test-mkt", ScopeLocal)
	require.NoError(t, err)

	// Workspace settings: should have chat.plugins.marketplaces + extraKnownMarketplaces
	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))

	arr, ok := ws["chat.plugins.marketplaces"].([]interface{})
	require.True(t, ok, "workspace: chat.plugins.marketplaces should be present")
	require.Len(t, arr, 1)
	uri := arr[0].(string)
	assert.Contains(t, uri, "file://")

	ekm, ok := ws["extraKnownMarketplaces"].(map[string]interface{})
	require.True(t, ok, "workspace: extraKnownMarketplaces should be present")
	entry := ekm["test-mkt"].(map[string]interface{})
	src := entry["source"].(map[string]interface{})
	assert.Equal(t, "directory", src["source"])

	// User-level settings: should have chat.plugins.marketplaces (application-scoped)
	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	uArr, ok := us["chat.plugins.marketplaces"].([]interface{})
	require.True(t, ok, "user settings: chat.plugins.marketplaces should be present")
	require.Len(t, uArr, 1)
	assert.Equal(t, uri, uArr[0].(string))

	// User-level should have chat.plugins.enabled = true (master toggle)
	enabled, ok := us["chat.plugins.enabled"].(bool)
	assert.True(t, ok && enabled, "user settings: chat.plugins.enabled should be true")

	// User-level should NOT have extraKnownMarketplaces
	_, hasEKM := us["extraKnownMarketplaces"]
	assert.False(t, hasEKM, "user settings: extraKnownMarketplaces should not be present")
}

func TestCopilotAdapter_Register_Idempotent(t *testing.T) {
	a, _, globalDir := newTestCopilotAdapter(t)

	require.NoError(t, a.Register("/store/copilot", "mkt", ScopeLocal))
	require.NoError(t, a.Register("/store/copilot", "mkt", ScopeLocal))

	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	arr := us["chat.plugins.marketplaces"].([]interface{})
	assert.Len(t, arr, 1, "should not duplicate marketplace entry in user settings")
}

func TestCopilotAdapter_Unregister_CleansAll(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)

	platformDir := filepath.Join(tmpDir, "platforms", "copilot")
	require.NoError(t, os.MkdirAll(platformDir, 0o755))
	require.NoError(t, a.Register(platformDir, "copilot", ScopeLocal))

	// Verify both settings were written
	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	_, hasEKM := ws["extraKnownMarketplaces"]
	require.True(t, hasEKM)
	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	_, hasMkt := us["chat.plugins.marketplaces"]
	require.True(t, hasMkt)

	// Unregister
	require.NoError(t, a.Unregister("copilot", ScopeLocal))

	wsAfter := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	_, hasMkt = wsAfter["chat.plugins.marketplaces"]
	assert.False(t, hasMkt, "workspace: chat.plugins.marketplaces should be removed")
	_, hasEKM = wsAfter["extraKnownMarketplaces"]
	assert.False(t, hasEKM, "workspace: extraKnownMarketplaces should be removed")

	usAfter := readSettings(t, filepath.Join(globalDir, "settings.json"))
	_, hasMkt = usAfter["chat.plugins.marketplaces"]
	assert.False(t, hasMkt, "user settings: chat.plugins.marketplaces should be removed")
}

func TestCopilotAdapter_Unregister_NoFile(t *testing.T) {
	a, _, _ := newTestCopilotAdapter(t)
	err := a.Unregister("nonexistent", ScopeLocal)
	assert.NoError(t, err)
}

func TestCopilotAdapter_EnablePlugin(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "my-plugin"), 0o755))

	err := a.EnablePlugin("my-plugin", "summon-local", storeDir, ScopeLocal)
	require.NoError(t, err)

	expectedPath := canonicalPluginPath(storeDir, "my-plugin")

	// Workspace: chat.pluginLocations + enabledPlugins
	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	pl := ws["chat.pluginLocations"].(map[string]interface{})
	assert.Equal(t, true, pl[expectedPath])
	ep := ws["enabledPlugins"].(map[string]interface{})
	assert.Equal(t, true, ep["my-plugin@summon-local"])

	// User-level: chat.pluginLocations only (application-scoped activation)
	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	uPl := us["chat.pluginLocations"].(map[string]interface{})
	assert.Equal(t, true, uPl[expectedPath])
	_, hasEP := us["enabledPlugins"]
	assert.False(t, hasEP, "user settings: enabledPlugins should not be present")
}

func TestCopilotAdapter_EnablePlugin_PreservesExisting(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "p1"), 0o755))

	require.NoError(t, a.Register("/store", "mkt", ScopeLocal))
	require.NoError(t, a.EnablePlugin("p1", "mkt", storeDir, ScopeLocal))

	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	_, hasMkt := ws["chat.plugins.marketplaces"]
	assert.True(t, hasMkt, "chat.plugins.marketplaces should still be present")
	_, hasPL := ws["chat.pluginLocations"]
	assert.True(t, hasPL, "chat.pluginLocations should be present")
}

func TestCopilotAdapter_DisablePlugin(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "p1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "p2"), 0o755))

	require.NoError(t, a.EnablePlugin("p1", "mkt", storeDir, ScopeLocal))
	require.NoError(t, a.EnablePlugin("p2", "mkt", storeDir, ScopeLocal))
	require.NoError(t, a.DisablePlugin("p1", "mkt", storeDir, ScopeLocal))

	// Workspace: p2 remains
	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	pl := ws["chat.pluginLocations"].(map[string]interface{})
	assert.Len(t, pl, 1)
	p2Path := canonicalPluginPath(storeDir, "p2")
	assert.Equal(t, true, pl[p2Path])
	ep := ws["enabledPlugins"].(map[string]interface{})
	assert.Len(t, ep, 1)
	assert.Equal(t, true, ep["p2@mkt"])

	// User-level: p2 remains
	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	uPl := us["chat.pluginLocations"].(map[string]interface{})
	assert.Len(t, uPl, 1)
	assert.Equal(t, true, uPl[p2Path])
}

func TestCopilotAdapter_DisablePlugin_RemovesKeyWhenEmpty(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "p"), 0o755))

	require.NoError(t, a.EnablePlugin("p", "mkt", storeDir, ScopeLocal))
	require.NoError(t, a.DisablePlugin("p", "mkt", storeDir, ScopeLocal))

	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	_, hasPL := ws["chat.pluginLocations"]
	assert.False(t, hasPL, "workspace: chat.pluginLocations should be removed when empty")
	_, hasEP := ws["enabledPlugins"]
	assert.False(t, hasEP, "workspace: enabledPlugins should be removed when empty")

	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	_, hasPL = us["chat.pluginLocations"]
	assert.False(t, hasPL, "user settings: chat.pluginLocations should be removed when empty")
}

func TestCopilotAdapter_DisablePlugin_NoFile(t *testing.T) {
	a, _, _ := newTestCopilotAdapter(t)
	err := a.DisablePlugin("nonexistent", "mkt", "/some/store", ScopeLocal)
	assert.NoError(t, err)
}

func TestCopilotAdapter_Register_Global(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)

	err := a.Register("/store/copilot", "test-mkt", ScopeGlobal)
	require.NoError(t, err)

	// User-level settings should have chat.plugins.marketplaces
	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	arr, ok := us["chat.plugins.marketplaces"].([]interface{})
	require.True(t, ok, "user settings: chat.plugins.marketplaces should be present")
	require.Len(t, arr, 1)

	// User-level should have chat.plugins.enabled = true
	enabled, ok := us["chat.plugins.enabled"].(bool)
	assert.True(t, ok && enabled, "user settings: chat.plugins.enabled should be true")

	// No workspace settings should be created for global scope
	wsPath := filepath.Join(tmpDir, ".vscode", "settings.json")
	_, err = os.Stat(wsPath)
	assert.True(t, os.IsNotExist(err), "workspace settings should not be created for global scope")
}

func TestCopilotAdapter_EnablePlugin_Global(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "my-plugin"), 0o755))

	err := a.EnablePlugin("my-plugin", "summon-global", storeDir, ScopeGlobal)
	require.NoError(t, err)

	expectedPath := canonicalPluginPath(storeDir, "my-plugin")

	// User-level settings should have chat.pluginLocations
	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	pl := us["chat.pluginLocations"].(map[string]interface{})
	assert.Equal(t, true, pl[expectedPath])

	// No workspace settings should be created for global scope
	wsPath := filepath.Join(tmpDir, ".vscode", "settings.json")
	_, err = os.Stat(wsPath)
	assert.True(t, os.IsNotExist(err), "workspace settings should not be created for global scope")

	// User settings should NOT have enabledPlugins (workspace-only key)
	_, hasEP := us["enabledPlugins"]
	assert.False(t, hasEP, "user settings should not have enabledPlugins for global scope")
}

func TestCopilotAdapter_Register_SetsPluginsEnabled(t *testing.T) {
	a, _, globalDir := newTestCopilotAdapter(t)

	// Pre-populate user settings with chat.plugins.enabled = false
	initial := map[string]interface{}{"chat.plugins.enabled": false, "other": "value"}
	writeJSONFile(filepath.Join(globalDir, "settings.json"), initial)

	require.NoError(t, a.Register("/store/copilot", "mkt", ScopeGlobal))

	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	enabled, ok := us["chat.plugins.enabled"].(bool)
	assert.True(t, ok && enabled, "chat.plugins.enabled should be flipped to true")
	assert.Equal(t, "value", us["other"], "existing settings should be preserved")
}

func TestCopilotAdapter_Register_PluginsEnabledIdempotent(t *testing.T) {
	a, _, globalDir := newTestCopilotAdapter(t)

	require.NoError(t, a.Register("/store/a", "mkt", ScopeGlobal))
	require.NoError(t, a.Register("/store/b", "mkt", ScopeGlobal))

	us := readSettings(t, filepath.Join(globalDir, "settings.json"))
	enabled, ok := us["chat.plugins.enabled"].(bool)
	assert.True(t, ok && enabled, "chat.plugins.enabled should still be true after second register")
}

func TestCopilotAdapter_Register_Project_WritesWorkspaceOnly(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)

	require.NoError(t, a.Register("/store/copilot", "proj-mkt", ScopeProject))

	// Workspace settings should have chat.plugins.marketplaces
	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	arr, ok := ws["chat.plugins.marketplaces"].([]interface{})
	require.True(t, ok, "workspace: chat.plugins.marketplaces should be present for ScopeProject")
	require.Len(t, arr, 1)
	assert.Contains(t, arr[0].(string), "file://")

	// User-level settings should NOT be written for project scope
	_, err := os.Stat(filepath.Join(globalDir, "settings.json"))
	assert.True(t, os.IsNotExist(err), "user settings should not be written for ScopeProject")
}

func TestCopilotAdapter_EnablePlugin_Project_WritesWorkspaceOnly(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "proj-plugin"), 0o755))

	require.NoError(t, a.EnablePlugin("proj-plugin", "summon-project", storeDir, ScopeProject))

	expectedPath := canonicalPluginPath(storeDir, "proj-plugin")

	// Workspace settings should have chat.pluginLocations
	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	pl, ok := ws["chat.pluginLocations"].(map[string]interface{})
	require.True(t, ok, "workspace: chat.pluginLocations should be present for ScopeProject")
	assert.Equal(t, true, pl[expectedPath])

	// User-level settings should NOT be written for project scope
	_, err := os.Stat(filepath.Join(globalDir, "settings.json"))
	assert.True(t, os.IsNotExist(err), "user settings should not be written for ScopeProject")
}

func TestCopilotAdapter_SettingsPath_Project(t *testing.T) {
	a := &CopilotAdapter{ProjectDir: "/my/project"}
	path := a.SettingsPath(ScopeProject)
	assert.Equal(t, filepath.Join("/my/project", ".vscode", "settings.json"), path)
}

func TestCopilotAdapter_MaterializeComponents_Project_Skills(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	// Create a realistic skill subdirectory structure: skills/my-skill/SKILL.md
	skillSubdir := filepath.Join(pkgDir, "skills", "my-skill")
	require.NoError(t, os.MkdirAll(skillSubdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillSubdir, "SKILL.md"), []byte("# my-skill"), 0o644))

	m := &testManifest{name: "my-pkg", skills: "skills"}
	err := a.MaterializeComponents(pkgDir, m, ScopeProject)
	require.NoError(t, err)

	// Each skill subdirectory should be linked individually under .github/skills/
	target := filepath.Join(tmpDir, ".github", "skills", "my-skill")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".github/skills/my-skill should be created")

	// SKILL.md must be at depth 1 from discovery root
	_, err = os.Stat(filepath.Join(target, "SKILL.md"))
	assert.NoError(t, err, "SKILL.md should be at depth 1: .github/skills/my-skill/SKILL.md")

	// The old package-named directory should NOT exist
	_, err = os.Lstat(filepath.Join(tmpDir, ".github", "skills", "my-pkg"))
	assert.True(t, os.IsNotExist(err), ".github/skills/my-pkg should NOT exist (old behavior)")
}

func TestCopilotAdapter_MaterializeComponents_Project_Agents(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	// Create a realistic agent subdirectory structure: agents/my-agent/my-agent.agent.md
	agentSubdir := filepath.Join(pkgDir, "agents", "my-agent")
	require.NoError(t, os.MkdirAll(agentSubdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentSubdir, "my-agent.agent.md"), []byte("# agent"), 0o644))

	m := &testManifest{name: "my-pkg", agents: "agents"}
	err := a.MaterializeComponents(pkgDir, m, ScopeProject)
	require.NoError(t, err)

	// Each agent subdirectory should be linked individually under .github/agents/
	target := filepath.Join(tmpDir, ".github", "agents", "my-agent")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".github/agents/my-agent should be created")

	// .agent.md must be at depth 1 from discovery root
	_, err = os.Stat(filepath.Join(target, "my-agent.agent.md"))
	assert.NoError(t, err, ".agent.md should be at depth 1: .github/agents/my-agent/my-agent.agent.md")

	// The old package-named directory should NOT exist
	_, err = os.Lstat(filepath.Join(tmpDir, ".github", "agents", "my-pkg"))
	assert.True(t, os.IsNotExist(err), ".github/agents/my-pkg should NOT exist (old behavior)")
}

func TestCopilotAdapter_MaterializeComponents_Local_Skills(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	skillSubdir := filepath.Join(pkgDir, "skills", "my-skill")
	require.NoError(t, os.MkdirAll(skillSubdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillSubdir, "SKILL.md"), []byte("# skill"), 0o644))

	m := &testManifest{name: "my-pkg", skills: "skills"}
	err := a.MaterializeComponents(pkgDir, m, ScopeLocal)
	require.NoError(t, err)

	// Skill subdirectory should be linked under .claude/skills/
	target := filepath.Join(tmpDir, ".claude", "skills", "my-skill")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".claude/skills/my-skill should be created for ScopeLocal")

	// SKILL.md must be at depth 1
	_, err = os.Stat(filepath.Join(target, "SKILL.md"))
	assert.NoError(t, err, "SKILL.md should be at depth 1: .claude/skills/my-skill/SKILL.md")
}

func TestCopilotAdapter_MaterializeComponents_Local_Agents(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	agentSubdir := filepath.Join(pkgDir, "agents", "my-agent")
	require.NoError(t, os.MkdirAll(agentSubdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentSubdir, "my-agent.agent.md"), []byte("# agent"), 0o644))

	m := &testManifest{name: "my-pkg", agents: "agents"}
	err := a.MaterializeComponents(pkgDir, m, ScopeLocal)
	require.NoError(t, err)

	target := filepath.Join(tmpDir, ".claude", "agents", "my-agent")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".claude/agents/my-agent should be created for ScopeLocal")

	_, err = os.Stat(filepath.Join(target, "my-agent.agent.md"))
	assert.NoError(t, err, ".agent.md should be at depth 1")
}

func TestCopilotAdapter_MaterializeComponents_Local_MCP_Fails(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	mcpDir := filepath.Join(pkgDir, "mcp")
	require.NoError(t, os.MkdirAll(mcpDir, 0o755))

	m := &testManifest{name: "my-pkg", mcp: "mcp"}
	err := a.MaterializeComponents(pkgDir, m, ScopeLocal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp")
	assert.Contains(t, err.Error(), "local")
}

func TestCopilotAdapter_RemoveMaterialized_Project(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	skillSubdir := filepath.Join(pkgDir, "skills", "my-skill")
	require.NoError(t, os.MkdirAll(skillSubdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillSubdir, "SKILL.md"), []byte("# skill"), 0o644))

	m := &testManifest{name: "my-pkg", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, ScopeProject))

	// Verify individual skill link exists
	target := filepath.Join(tmpDir, ".github", "skills", "my-skill")
	_, err := os.Lstat(target)
	require.NoError(t, err, "link should exist before removal")

	require.NoError(t, a.RemoveMaterialized("my-pkg", pkgDir, m, ScopeProject))
	_, err = os.Lstat(target)
	assert.True(t, os.IsNotExist(err), "individual skill link should be removed")
}

func TestCopilotAdapter_MaterializeComponents_MultiSkill(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-tools")
	for _, skill := range []string{"linter", "formatter"} {
		subdir := filepath.Join(pkgDir, "skills", skill)
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "SKILL.md"), []byte("# "+skill), 0o644))
	}

	m := &testManifest{name: "my-tools", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, ScopeLocal))

	// Both skills should be individually linked
	for _, skill := range []string{"linter", "formatter"} {
		target := filepath.Join(tmpDir, ".claude", "skills", skill)
		_, err := os.Lstat(target)
		assert.NoError(t, err, ".claude/skills/%s should exist", skill)

		_, err = os.Stat(filepath.Join(target, "SKILL.md"))
		assert.NoError(t, err, "SKILL.md should be at depth 1 for %s", skill)
	}

	// Package-named directory should NOT exist
	_, err := os.Lstat(filepath.Join(tmpDir, ".claude", "skills", "my-tools"))
	assert.True(t, os.IsNotExist(err), "package-named directory should not exist")
}

func TestCopilotAdapter_RemoveMaterialized_MultiSkill(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-tools")
	for _, skill := range []string{"linter", "formatter"} {
		subdir := filepath.Join(pkgDir, "skills", skill)
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "SKILL.md"), []byte("# "+skill), 0o644))
	}

	m := &testManifest{name: "my-tools", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, ScopeProject))

	// Both should exist
	for _, skill := range []string{"linter", "formatter"} {
		_, err := os.Lstat(filepath.Join(tmpDir, ".github", "skills", skill))
		require.NoError(t, err)
	}

	// Remove and verify both are gone
	require.NoError(t, a.RemoveMaterialized("my-tools", pkgDir, m, ScopeProject))
	for _, skill := range []string{"linter", "formatter"} {
		_, err := os.Lstat(filepath.Join(tmpDir, ".github", "skills", skill))
		assert.True(t, os.IsNotExist(err), "%s link should be removed", skill)
	}
}

func TestCopilotAdapter_MaterializeComponents_MultiAgent(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-agents")
	for _, agent := range []string{"reviewer", "fixer"} {
		subdir := filepath.Join(pkgDir, "agents", agent)
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, agent+".agent.md"), []byte("# "+agent), 0o644))
	}

	m := &testManifest{name: "my-agents", agents: "agents"}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, ScopeProject))

	for _, agent := range []string{"reviewer", "fixer"} {
		target := filepath.Join(tmpDir, ".github", "agents", agent)
		_, err := os.Lstat(target)
		assert.NoError(t, err, ".github/agents/%s should exist", agent)

		_, err = os.Stat(filepath.Join(target, agent+".agent.md"))
		assert.NoError(t, err, ".agent.md should be at depth 1 for %s", agent)
	}
}

func TestCopilotAdapter_RemoveMaterialized_Agents(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-agents")
	agentSubdir := filepath.Join(pkgDir, "agents", "my-agent")
	require.NoError(t, os.MkdirAll(agentSubdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentSubdir, "my-agent.agent.md"), []byte("# agent"), 0o644))

	m := &testManifest{name: "my-agents", agents: "agents"}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, ScopeLocal))

	target := filepath.Join(tmpDir, ".claude", "agents", "my-agent")
	_, err := os.Lstat(target)
	require.NoError(t, err)

	require.NoError(t, a.RemoveMaterialized("my-agents", pkgDir, m, ScopeLocal))
	_, err = os.Lstat(target)
	assert.True(t, os.IsNotExist(err), "agent link should be removed")
}

func TestCopilotAdapter_RemoveMaterialized_CollisionSafe(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	// Package A has a "review" skill
	pkgDirA := filepath.Join(tmpDir, "store", "pkg-a")
	subdirA := filepath.Join(pkgDirA, "skills", "review")
	require.NoError(t, os.MkdirAll(subdirA, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdirA, "SKILL.md"), []byte("# A"), 0o644))

	mA := &testManifest{name: "pkg-a", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDirA, mA, ScopeLocal))

	// Package B also has a "review" skill — overwrites A's link
	pkgDirB := filepath.Join(tmpDir, "store", "pkg-b")
	subdirB := filepath.Join(pkgDirB, "skills", "review")
	require.NoError(t, os.MkdirAll(subdirB, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdirB, "SKILL.md"), []byte("# B"), 0o644))

	mB := &testManifest{name: "pkg-b", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDirB, mB, ScopeLocal))

	// Uninstall pkg-a — the link now points to pkg-b, so it must NOT be removed
	require.NoError(t, a.RemoveMaterialized("pkg-a", pkgDirA, mA, ScopeLocal))

	link := filepath.Join(tmpDir, ".claude", "skills", "review")
	_, err := os.Lstat(link)
	assert.NoError(t, err, "link should still exist — it belongs to pkg-b now")

	resolved, err := os.Readlink(link)
	require.NoError(t, err)
	absResolved, _ := filepath.Abs(resolved)
	absB, _ := filepath.Abs(subdirB)
	assert.Equal(t, absB, absResolved, "link should still point to pkg-b's review skill")
}

func TestCopilotAdapter_MaterializeComponents_CollisionWarning(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	// Package A has a "review" skill
	pkgDirA := filepath.Join(tmpDir, "store", "pkg-a")
	subdirA := filepath.Join(pkgDirA, "skills", "review")
	require.NoError(t, os.MkdirAll(subdirA, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdirA, "SKILL.md"), []byte("# A"), 0o644))

	mA := &testManifest{name: "pkg-a", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDirA, mA, ScopeLocal))

	// Package B also has a "review" skill
	pkgDirB := filepath.Join(tmpDir, "store", "pkg-b")
	subdirB := filepath.Join(pkgDirB, "skills", "review")
	require.NoError(t, os.MkdirAll(subdirB, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdirB, "SKILL.md"), []byte("# B"), 0o644))

	mB := &testManifest{name: "pkg-b", skills: "skills"}
	// This should succeed (overwrite) — collision warning is printed to stderr
	require.NoError(t, a.MaterializeComponents(pkgDirB, mB, ScopeLocal))

	// Verify the link now points to package B's skill
	target := filepath.Join(tmpDir, ".claude", "skills", "review")
	resolved, err := os.Readlink(target)
	require.NoError(t, err)
	absResolved, _ := filepath.Abs(resolved)
	absDirB, _ := filepath.Abs(subdirB)
	assert.Equal(t, absDirB, absResolved, "link should point to pkg-b's review skill")
}

func TestCopilotAdapter_MaterializeComponents_EmptySkillsDir(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "empty-pkg")
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "skills"), 0o755))

	m := &testManifest{name: "empty-pkg", skills: "skills"}
	err := a.MaterializeComponents(pkgDir, m, ScopeLocal)
	assert.NoError(t, err, "empty skills directory should not cause an error")
}

func TestCopilotAdapter_MaterializeComponents_MissingComponentDir(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "missing-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	// Do NOT create the skills directory

	m := &testManifest{name: "missing-pkg", skills: "skills"}
	err := a.MaterializeComponents(pkgDir, m, ScopeLocal)
	assert.Error(t, err, "missing component directory should return an error")
}

// testManifest is a simple component carrier used in materialization tests.
type testManifest struct {
	name   string
	skills string
	agents string
	hooks  string
	mcp    string
}

func (m *testManifest) GetName() string   { return m.name }
func (m *testManifest) GetSkills() string { return m.skills }
func (m *testManifest) GetAgents() string { return m.agents }
func (m *testManifest) GetHooks() string  { return m.hooks }
func (m *testManifest) GetMCP() string    { return m.mcp }

// --- T014: Parse-failure preservation test for Copilot adapter ---

func TestCopilotAdapter_Register_ParseFailurePreservesFile(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	// Create workspace settings with JSONC content
	vsDir := filepath.Join(tmpDir, ".vscode")
	require.NoError(t, os.MkdirAll(vsDir, 0o755))
	wsPath := filepath.Join(vsDir, "settings.json")
	jsoncContent := []byte(`{
  // User comment
  "editor.fontSize": 14,
  "theme": "dark"
}`)
	require.NoError(t, os.WriteFile(wsPath, jsoncContent, 0o644))

	err := a.Register("/store/copilot", "test-mkt", ScopeProject)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")

	// File must be byte-identical
	after, err := os.ReadFile(wsPath)
	require.NoError(t, err)
	assert.Equal(t, jsoncContent, after)
}

func TestCopilotAdapter_EnablePlugin_ParseFailurePreservesFile(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "my-plugin"), 0o755))

	vsDir := filepath.Join(tmpDir, ".vscode")
	require.NoError(t, os.MkdirAll(vsDir, 0o755))
	wsPath := filepath.Join(vsDir, "settings.json")
	jsoncContent := []byte(`{
  // comment
  "editor.fontSize": 14
}`)
	require.NoError(t, os.WriteFile(wsPath, jsoncContent, 0o644))

	err := a.EnablePlugin("my-plugin", "mkt", storeDir, ScopeProject)
	require.Error(t, err)

	after, err := os.ReadFile(wsPath)
	require.NoError(t, err)
	assert.Equal(t, jsoncContent, after)
}

// --- T016: Non-destructive merge regression test for Copilot adapter ---

func TestCopilotAdapter_NonDestructiveMerge(t *testing.T) {
	a, tmpDir, globalDir := newTestCopilotAdapter(t)
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "p1"), 0o755))

	// Pre-populate workspace settings with many unrelated keys
	vsDir := filepath.Join(tmpDir, ".vscode")
	require.NoError(t, os.MkdirAll(vsDir, 0o755))
	wsPath := filepath.Join(vsDir, "settings.json")

	wsOriginal := map[string]interface{}{
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
	require.NoError(t, writeJSONFile(wsPath, wsOriginal))

	// Pre-populate global settings
	globalOriginal := map[string]interface{}{
		"editor.fontSize":      float64(16),
		"workbench.colorTheme": "Solarized",
		"explorer.sortOrder":   "type",
	}
	require.NoError(t, writeJSONFile(filepath.Join(globalDir, "settings.json"), globalOriginal))

	// Full install cycle
	require.NoError(t, a.Register("/store/copilot", "mkt", ScopeLocal))
	require.NoError(t, a.EnablePlugin("p1", "mkt", storeDir, ScopeLocal))
	require.NoError(t, a.DisablePlugin("p1", "mkt", storeDir, ScopeLocal))
	require.NoError(t, a.Unregister("mkt", ScopeLocal))

	// Read back workspace settings and verify all original keys survive
	wsAfter := readSettings(t, wsPath)
	for key, expected := range wsOriginal {
		assert.Equal(t, expected, wsAfter[key], "workspace key %q should be preserved", key)
	}

	// Read back global settings and verify original keys survive
	globalAfter := readSettings(t, filepath.Join(globalDir, "settings.json"))
	for key, expected := range globalOriginal {
		assert.Equal(t, expected, globalAfter[key], "global key %q should be preserved", key)
	}
}

// --- T018: Atomic write integration test for Copilot adapter ---

func TestCopilotAdapter_Register_AtomicWrite(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	require.NoError(t, a.Register("/store/copilot", "test-mkt", ScopeProject))

	vsDir := filepath.Join(tmpDir, ".vscode")
	wsPath := filepath.Join(vsDir, "settings.json")

	// Output should be valid JSON
	data, err := os.ReadFile(wsPath)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	// No temp files should remain
	entries, err := os.ReadDir(vsDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.Contains(e.Name(), ".summon-settings-"),
			"temp file should be cleaned up: %s", e.Name())
	}
}

// --- T021: Scope-specific parse-failure test for Copilot adapter ---

func TestCopilotAdapter_ParseFailure_AllScopes(t *testing.T) {
	jsoncContent := []byte(`{
  // comment
  "key": "value"
}`)

	t.Run("ScopeProject_workspace", func(t *testing.T) {
		a, tmpDir, _ := newTestCopilotAdapter(t)
		vsDir := filepath.Join(tmpDir, ".vscode")
		require.NoError(t, os.MkdirAll(vsDir, 0o755))
		wsPath := filepath.Join(vsDir, "settings.json")
		require.NoError(t, os.WriteFile(wsPath, jsoncContent, 0o644))

		err := a.Register("/store", "mkt", ScopeProject)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not valid JSON")

		after, err := os.ReadFile(wsPath)
		require.NoError(t, err)
		assert.Equal(t, jsoncContent, after)
	})

	t.Run("ScopeLocal_workspace", func(t *testing.T) {
		a, tmpDir, _ := newTestCopilotAdapter(t)
		vsDir := filepath.Join(tmpDir, ".vscode")
		require.NoError(t, os.MkdirAll(vsDir, 0o755))
		wsPath := filepath.Join(vsDir, "settings.json")
		require.NoError(t, os.WriteFile(wsPath, jsoncContent, 0o644))

		err := a.Register("/store", "mkt", ScopeLocal)
		require.Error(t, err)

		after, err := os.ReadFile(wsPath)
		require.NoError(t, err)
		assert.Equal(t, jsoncContent, after)
	})

	t.Run("ScopeGlobal_userLevel", func(t *testing.T) {
		a, _, globalDir := newTestCopilotAdapter(t)
		globalPath := filepath.Join(globalDir, "settings.json")
		require.NoError(t, os.WriteFile(globalPath, jsoncContent, 0o644))

		err := a.Register("/store", "mkt", ScopeGlobal)
		require.Error(t, err)

		after, err := os.ReadFile(globalPath)
		require.NoError(t, err)
		assert.Equal(t, jsoncContent, after)
	})
}

// --- T022: Missing-file creation test for Copilot adapter ---

func TestCopilotAdapter_MissingFile_CreatesWithRequiredKeys(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	// File does not exist — Register should create it
	err := a.Register("/store", "mkt", ScopeProject)
	require.NoError(t, err)

	ws := readSettings(t, filepath.Join(tmpDir, ".vscode", "settings.json"))
	_, hasMkt := ws["chat.plugins.marketplaces"]
	assert.True(t, hasMkt, "should have chat.plugins.marketplaces")
	_, hasEKM := ws["extraKnownMarketplaces"]
	assert.True(t, hasEKM, "should have extraKnownMarketplaces")
}

// --- T024: Error message format test for Copilot adapter ---

func TestCopilotAdapter_Register_ErrorMessageFormat(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	vsDir := filepath.Join(tmpDir, ".vscode")
	require.NoError(t, os.MkdirAll(vsDir, 0o755))
	wsPath := filepath.Join(vsDir, "settings.json")
	require.NoError(t, os.WriteFile(wsPath, []byte(`{// comment}`), 0o644))

	err := a.Register("/store", "mkt", ScopeProject)
	require.Error(t, err)
	errMsg := err.Error()
	assert.Contains(t, errMsg, wsPath)
	assert.Contains(t, errMsg, "not valid JSON")
	assert.Contains(t, errMsg, "comments")
}

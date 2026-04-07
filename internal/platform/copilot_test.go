package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	skillsDir := filepath.Join(pkgDir, "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill.md"), []byte("# skill"), 0o644))

	m := &testManifest{name: "my-pkg", skills: "skills"}
	err := a.MaterializeComponents(pkgDir, m, ScopeProject)
	require.NoError(t, err)

	// Skills should be linked under .github/skills/my-pkg
	target := filepath.Join(tmpDir, ".github", "skills", "my-pkg")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".github/skills/my-pkg should be created")
}

func TestCopilotAdapter_MaterializeComponents_Project_Agents(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	agentsDir := filepath.Join(pkgDir, "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "agent.md"), []byte("# agent"), 0o644))

	m := &testManifest{name: "my-pkg", agents: "agents"}
	err := a.MaterializeComponents(pkgDir, m, ScopeProject)
	require.NoError(t, err)

	// Agents should be linked under .github/agents/my-pkg
	target := filepath.Join(tmpDir, ".github", "agents", "my-pkg")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".github/agents/my-pkg should be created")
}

func TestCopilotAdapter_MaterializeComponents_Local_Skills(t *testing.T) {
	a, tmpDir, _ := newTestCopilotAdapter(t)

	pkgDir := filepath.Join(tmpDir, "store", "my-pkg")
	skillsDir := filepath.Join(pkgDir, "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill.md"), []byte("# skill"), 0o644))

	m := &testManifest{name: "my-pkg", skills: "skills"}
	err := a.MaterializeComponents(pkgDir, m, ScopeLocal)
	require.NoError(t, err)

	// Skills should be linked under .claude/skills/my-pkg for local scope
	target := filepath.Join(tmpDir, ".claude", "skills", "my-pkg")
	_, err = os.Lstat(target)
	assert.NoError(t, err, ".claude/skills/my-pkg should be created for ScopeLocal")
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
	skillsDir := filepath.Join(pkgDir, "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))

	m := &testManifest{name: "my-pkg", skills: "skills"}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, ScopeProject))

	target := filepath.Join(tmpDir, ".github", "skills", "my-pkg")
	_, err := os.Lstat(target)
	require.NoError(t, err, "link should exist before removal")

	require.NoError(t, a.RemoveMaterialized("my-pkg", m, ScopeProject))
	_, err = os.Lstat(target)
	assert.True(t, os.IsNotExist(err), "link should be removed")
}

// testManifest is a simple component carrier used in materialization tests.
type testManifest struct {
	name   string
	skills string
	agents string
	hooks  string
	mcp    string
}

func (m *testManifest) GetName() string    { return m.name }
func (m *testManifest) GetSkills() string  { return m.skills }
func (m *testManifest) GetAgents() string  { return m.agents }
func (m *testManifest) GetHooks() string   { return m.hooks }
func (m *testManifest) GetMCP() string     { return m.mcp }

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/skillscan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper: create plugin directories with skills ---

func createTestPluginDir(t *testing.T, baseDir, pluginName string, skills map[string]string) string {
	t.Helper()
	pluginDir := filepath.Join(baseDir, pluginName)
	skillsDir := filepath.Join(pluginDir, "skills")
	for skillName, frontmatter := range skills {
		d := filepath.Join(skillsDir, skillName)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(frontmatter), 0o644))
	}
	// Create minimal plugin.json
	cpDir := filepath.Join(pluginDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	pjData := `{"name":"` + pluginName + `"}`
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(pjData), 0o644))
	return pluginDir
}

type testPluginEntry struct {
	Name        string
	Marketplace string
	CachePath   string
	Enabled     *bool // nil = omit from JSON (defaults to enabled); non-nil = explicit
}

func writeCopilotConfig(t *testing.T, homeDir string, plugins []testPluginEntry) {
	t.Helper()
	type entry struct {
		Name        string `json:"name"`
		Marketplace string `json:"marketplace"`
		CachePath   string `json:"cache_path"`
		Enabled     *bool  `json:"enabled,omitempty"`
	}
	var entries []entry
	for _, p := range plugins {
		e := entry{
			Name:        p.Name,
			Marketplace: p.Marketplace,
			CachePath:   p.CachePath,
		}
		if p.Enabled != nil {
			e.Enabled = p.Enabled
		} else {
			e.Enabled = boolPtr(true)
		}
		entries = append(entries, e)
	}
	cfg := struct {
		InstalledPlugins []entry `json:"installedPlugins"`
	}{InstalledPlugins: entries}
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	copilotDir := filepath.Join(homeDir, ".copilot")
	require.NoError(t, os.MkdirAll(copilotDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(copilotDir, "config.json"), data, 0o644))
}

// --- printCollisions tests ---

func TestPrintCollisions_Empty(t *testing.T) {
	var buf bytes.Buffer
	s := NewStyles(true) // no color
	result := &collisionResult{CLI: "copilot"}
	printCollisions(&buf, result, s)
	assert.Empty(t, buf.String())
}

func TestPrintCollisions_SingleCollision(t *testing.T) {
	var buf bytes.Buffer
	s := NewStyles(true) // no color
	result := &collisionResult{
		CLI: "copilot",
		Collisions: []skillscan.Collision{
			{
				SkillName: "init",
				Entries: []skillscan.SkillEntry{
					{Name: "init", PluginName: "wingman", Marketplace: "summon-marketplace", FilePath: "skills/init/SKILL.md", Order: 0},
					{Name: "init", PluginName: "speckit", Marketplace: "summon-marketplace", FilePath: "skills/init/SKILL.md", Order: 2},
				},
			},
		},
		PluginCount: 3,
	}
	printCollisions(&buf, result, s)

	output := buf.String()
	assert.Contains(t, output, "Skill name collisions detected")
	assert.Contains(t, output, `"init"`)
	assert.Contains(t, output, "wingman")
	assert.Contains(t, output, "speckit")
	assert.Contains(t, output, "WINS")
	assert.Contains(t, output, "SHADOWED")
	assert.Contains(t, output, "1 collision found")
}

func TestPrintCollisions_MultipleCollisions(t *testing.T) {
	var buf bytes.Buffer
	s := NewStyles(true)
	result := &collisionResult{
		CLI: "copilot",
		Collisions: []skillscan.Collision{
			{
				SkillName: "brainstorm",
				Entries: []skillscan.SkillEntry{
					{Name: "brainstorm", PluginName: "wingman", Order: 0},
					{Name: "brainstorm", PluginName: "brainstorm-plugin", Order: 1},
				},
			},
			{
				SkillName: "init",
				Entries: []skillscan.SkillEntry{
					{Name: "init", PluginName: "wingman", Order: 0},
					{Name: "init", PluginName: "speckit", Order: 2},
				},
			},
		},
		PluginCount: 5,
	}
	printCollisions(&buf, result, s)

	output := buf.String()
	assert.Contains(t, output, "2 collisions found")
}

// --- collisionsToJSON tests ---

func TestCollisionsToJSON(t *testing.T) {
	collisions := []skillscan.Collision{
		{
			SkillName: "init",
			Entries: []skillscan.SkillEntry{
				{Name: "init", PluginName: "wingman", Marketplace: "summon-marketplace", FilePath: "skills/init/SKILL.md", Order: 0},
				{Name: "init", PluginName: "speckit", Marketplace: "summon-marketplace", FilePath: "skills/init/SKILL.md", Order: 2},
			},
		},
	}

	result := collisionsToJSON(collisions)
	require.Len(t, result, 1)
	assert.Equal(t, "init", result[0].SkillName)
	require.Len(t, result[0].Entries, 2)
	assert.Equal(t, "wins", result[0].Entries[0].Status)
	assert.Equal(t, "wingman", result[0].Entries[0].PluginName)
	assert.Equal(t, "shadowed", result[0].Entries[1].Status)
	assert.Equal(t, "speckit", result[0].Entries[1].PluginName)
}

// --- printInstallCollisionWarning tests ---

func TestPrintInstallCollisionWarning_NoNewPackageInvolved(t *testing.T) {
	var buf bytes.Buffer
	s := NewStyles(true)
	result := &collisionResult{
		CLI: "copilot",
		Collisions: []skillscan.Collision{
			{
				SkillName: "init",
				Entries: []skillscan.SkillEntry{
					{Name: "init", PluginName: "wingman", Order: 0},
					{Name: "init", PluginName: "speckit", Order: 1},
				},
			},
		},
		PluginCount: 3,
	}

	// New packages don't match any collision participants
	printInstallCollisionWarning(&buf, result, []string{"other-plugin"}, s)
	assert.Empty(t, buf.String())
}

func TestPrintInstallCollisionWarning_NewPackageInvolved(t *testing.T) {
	var buf bytes.Buffer
	s := NewStyles(true)
	result := &collisionResult{
		CLI: "copilot",
		Collisions: []skillscan.Collision{
			{
				SkillName: "init",
				Entries: []skillscan.SkillEntry{
					{Name: "init", PluginName: "wingman", Order: 0},
					{Name: "init", PluginName: "speckit", Order: 1},
				},
			},
		},
		PluginCount: 3,
	}

	printInstallCollisionWarning(&buf, result, []string{"speckit"}, s)
	output := buf.String()
	assert.Contains(t, output, "init")
	assert.Contains(t, output, "SHADOWED")
}

// --- scanPlatformCollisions integration tests ---

func TestScanPlatformCollisions_CopilotWithCollisions(t *testing.T) {
	homeDir := t.TempDir()
	pluginsBase := filepath.Join(homeDir, ".copilot", "installed-plugins", "mkt")
	require.NoError(t, os.MkdirAll(pluginsBase, 0o755))

	// Create two plugins with colliding "init" skill
	wingmanDir := createTestPluginDir(t, pluginsBase, "wingman", map[string]string{
		"init":       "---\nname: init\n---\n",
		"brainstorm": "---\nname: brainstorm\n---\n",
	})
	speckitDir := createTestPluginDir(t, pluginsBase, "speckit", map[string]string{
		"init": "---\nname: init\n---\n",
	})

	writeCopilotConfig(t, homeDir, []testPluginEntry{
		{Name: "wingman", Marketplace: "mkt", CachePath: wingmanDir},
		{Name: "speckit", Marketplace: "mkt", CachePath: speckitDir},
	})

	adapter := newFakeAdapter("copilot")
	result := scanPlatformCollisions(adapter, "user", &collisionCheckDeps{homeDir: homeDir})

	assert.Equal(t, 2, result.PluginCount)
	require.Len(t, result.Collisions, 1)
	assert.Equal(t, "init", result.Collisions[0].SkillName)
	assert.Equal(t, "wingman", result.Collisions[0].Entries[0].PluginName)
	assert.Equal(t, "speckit", result.Collisions[0].Entries[1].PluginName)
}

func TestScanPlatformCollisions_NoCollisions(t *testing.T) {
	homeDir := t.TempDir()
	pluginsBase := filepath.Join(homeDir, ".copilot", "installed-plugins", "mkt")
	require.NoError(t, os.MkdirAll(pluginsBase, 0o755))

	pluginADir := createTestPluginDir(t, pluginsBase, "plugin-a", map[string]string{
		"skill-a": "---\nname: skill-a\n---\n",
	})
	pluginBDir := createTestPluginDir(t, pluginsBase, "plugin-b", map[string]string{
		"skill-b": "---\nname: skill-b\n---\n",
	})

	writeCopilotConfig(t, homeDir, []testPluginEntry{
		{Name: "plugin-a", Marketplace: "mkt", CachePath: pluginADir},
		{Name: "plugin-b", Marketplace: "mkt", CachePath: pluginBDir},
	})

	adapter := newFakeAdapter("copilot")
	result := scanPlatformCollisions(adapter, "user", &collisionCheckDeps{homeDir: homeDir})

	assert.Equal(t, 2, result.PluginCount)
	assert.Empty(t, result.Collisions)
}

func TestScanPlatformCollisions_DisabledPlugin(t *testing.T) {
	homeDir := t.TempDir()
	pluginsBase := filepath.Join(homeDir, ".copilot", "installed-plugins", "mkt")
	require.NoError(t, os.MkdirAll(pluginsBase, 0o755))

	wingmanDir := createTestPluginDir(t, pluginsBase, "wingman", map[string]string{
		"init": "---\nname: init\n---\n",
	})
	speckitDir := createTestPluginDir(t, pluginsBase, "speckit", map[string]string{
		"init": "---\nname: init\n---\n",
	})

	// Write config with speckit disabled
	writeCopilotConfig(t, homeDir, []testPluginEntry{
		{Name: "wingman", Marketplace: "mkt", CachePath: wingmanDir, Enabled: boolPtr(true)},
		{Name: "speckit", Marketplace: "mkt", CachePath: speckitDir, Enabled: boolPtr(false)},
	})

	adapter := newFakeAdapter("copilot")
	result := scanPlatformCollisions(adapter, "user", &collisionCheckDeps{homeDir: homeDir})

	assert.Equal(t, 1, result.PluginCount) // only wingman is active
	assert.Empty(t, result.Collisions)
}

func TestScanPlatformCollisions_NoConfigFile(t *testing.T) {
	homeDir := t.TempDir()
	adapter := newFakeAdapter("copilot")
	result := scanPlatformCollisions(adapter, "user", &collisionCheckDeps{homeDir: homeDir})

	assert.Equal(t, 0, result.PluginCount)
	assert.Empty(t, result.Collisions)
}

func TestScanPlatformCollisions_GenericAdapter(t *testing.T) {
	homeDir := t.TempDir()

	// Create plugin dirs for the generic adapter path
	pluginDir := createTestPluginDir(t, homeDir, "my-plugin", map[string]string{
		"test-skill": "---\nname: test-skill\n---\n",
	})

	adapter := newFakeAdapter("unknown-cli")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin", Platform: "unknown-cli"},
		}, nil
	}
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return pluginDir, nil
	}

	result := scanPlatformCollisions(adapter, "user", &collisionCheckDeps{homeDir: homeDir})

	assert.Equal(t, 1, result.PluginCount)
	assert.Empty(t, result.Collisions)
}

// --- Check command integration with collisions ---

func TestCheck_WithCollisions(t *testing.T) {
	homeDir := t.TempDir()
	pluginsBase := filepath.Join(homeDir, ".copilot", "installed-plugins", "mkt")
	require.NoError(t, os.MkdirAll(pluginsBase, 0o755))

	wingmanDir := createTestPluginDir(t, pluginsBase, "wingman", map[string]string{
		"init": "---\nname: init\n---\n",
	})
	speckitDir := createTestPluginDir(t, pluginsBase, "speckit", map[string]string{
		"init": "---\nname: init\n---\n",
	})

	writeCopilotConfig(t, homeDir, []testPluginEntry{
		{Name: "wingman", Marketplace: "mkt", CachePath: wingmanDir},
		{Name: "speckit", Marketplace: "mkt", CachePath: speckitDir},
	})

	adapter := newFakeAdapter("copilot")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "wingman", Platform: "copilot"},
			{Name: "speckit", Platform: "copilot"},
		}, nil
	}

	var stdout bytes.Buffer
	deps := &checkDeps{
		runner:        newFakeRunner(),
		fetcher:       newFakeFetcher(),
		adapters:      []platform.Adapter{adapter},
		stdout:        &stdout,
		noColor:       true,
		collisionDeps: &collisionCheckDeps{homeDir: homeDir},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Skill name collisions detected")
	assert.Contains(t, output, "init")
	assert.Contains(t, output, "wingman")
	assert.Contains(t, output, "speckit")
	assert.Contains(t, output, "WINS")
	assert.Contains(t, output, "SHADOWED")
}

func TestCheck_JSONWithCollisions(t *testing.T) {
	homeDir := t.TempDir()
	pluginsBase := filepath.Join(homeDir, ".copilot", "installed-plugins", "mkt")
	require.NoError(t, os.MkdirAll(pluginsBase, 0o755))

	wingmanDir := createTestPluginDir(t, pluginsBase, "wingman", map[string]string{
		"init": "---\nname: init\n---\n",
	})
	speckitDir := createTestPluginDir(t, pluginsBase, "speckit", map[string]string{
		"init": "---\nname: init\n---\n",
	})

	writeCopilotConfig(t, homeDir, []testPluginEntry{
		{Name: "wingman", Marketplace: "mkt", CachePath: wingmanDir},
		{Name: "speckit", Marketplace: "mkt", CachePath: speckitDir},
	})

	adapter := newFakeAdapter("copilot")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "wingman", Platform: "copilot"},
			{Name: "speckit", Platform: "copilot"},
		}, nil
	}

	var stdout bytes.Buffer
	deps := &checkDeps{
		runner:        newFakeRunner(),
		fetcher:       newFakeFetcher(),
		adapters:      []platform.Adapter{adapter},
		stdout:        &stdout,
		noColor:       true,
		collisionDeps: &collisionCheckDeps{homeDir: homeDir},
	}

	checkJSON = true
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err)

	var parsed struct {
		Results         map[string]interface{} `json:"results"`
		SkillCollisions map[string]interface{} `json:"skill_collisions"`
	}
	err = json.Unmarshal(stdout.Bytes(), &parsed)
	require.NoError(t, err, "output should be valid JSON: %s", stdout.String())
	assert.Contains(t, parsed.SkillCollisions, "copilot")
}

func TestCheck_NoCollisions_CleanOutput(t *testing.T) {
	homeDir := t.TempDir()
	pluginsBase := filepath.Join(homeDir, ".copilot", "installed-plugins", "mkt")
	require.NoError(t, os.MkdirAll(pluginsBase, 0o755))

	pluginDir := createTestPluginDir(t, pluginsBase, "my-plugin", map[string]string{
		"unique-skill": "---\nname: unique-skill\n---\n",
	})

	writeCopilotConfig(t, homeDir, []testPluginEntry{
		{Name: "my-plugin", Marketplace: "mkt", CachePath: pluginDir},
	})

	adapter := newFakeAdapter("copilot")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "copilot"},
		}, nil
	}

	var stdout bytes.Buffer
	deps := &checkDeps{
		runner:        newFakeRunner(),
		fetcher:       newFakeFetcher(),
		adapters:      []platform.Adapter{adapter},
		stdout:        &stdout,
		noColor:       true,
		collisionDeps: &collisionCheckDeps{homeDir: homeDir},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err)

	output := stdout.String()
	assert.NotContains(t, output, "collision")
}

package installer

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func canonicalPluginPath(storeDir, name string) string {
	p, _ := filepath.Abs(filepath.Join(storeDir, name))
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(resolved, name)
	}
	return p
}

// ---------------------------------------------------------------------------
// ResolvePaths
// ---------------------------------------------------------------------------

func TestResolvePaths_Local(t *testing.T) {
	projectDir := "/projects/myapp"
	p := ResolvePaths(platform.ScopeLocal, projectDir)

	assert.Equal(t, filepath.Join(projectDir, ".summon", "local", "store"), p.StoreDir)
	assert.Equal(t, filepath.Join(projectDir, ".summon", "local", "registry.yaml"), p.RegistryPath)
	assert.Equal(t, filepath.Join(projectDir, ".summon", "local", "platforms"), p.PlatformsDir)
	assert.Equal(t, platform.ScopeLocal, p.Scope)
	assert.Equal(t, "summon-local", p.MarketplaceName)
}

func TestResolvePaths_Project(t *testing.T) {
	projectDir := "/projects/myapp"
	p := ResolvePaths(platform.ScopeProject, projectDir)

	assert.Equal(t, filepath.Join(projectDir, ".summon", "project", "store"), p.StoreDir)
	assert.Equal(t, filepath.Join(projectDir, ".summon", "project", "registry.yaml"), p.RegistryPath)
	assert.Equal(t, filepath.Join(projectDir, ".summon", "project", "platforms"), p.PlatformsDir)
	assert.Equal(t, platform.ScopeProject, p.Scope)
	assert.Equal(t, "summon-project", p.MarketplaceName)
}

func TestResolvePaths_User(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	p := ResolvePaths(platform.ScopeUser, "/ignored")

	assert.Equal(t, filepath.Join(home, ".summon", "user", "store"), p.StoreDir)
	assert.Equal(t, filepath.Join(home, ".summon", "user", "registry.yaml"), p.RegistryPath)
	assert.Equal(t, filepath.Join(home, ".summon", "user", "platforms"), p.PlatformsDir)
	assert.Equal(t, platform.ScopeUser, p.Scope)
	assert.Equal(t, "summon-user", p.MarketplaceName)
}

// ---------------------------------------------------------------------------
// packageNameFromURL
// ---------------------------------------------------------------------------

func TestPackageNameFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"https URL", "https://github.com/user/repo", "repo"},
		{"https URL with .git", "https://github.com/user/repo.git", "repo"},
		{"ssh URL with .git", "git@github.com:user/repo.git", "repo"},
		{"single segment", "mypkg", "mypkg"},
		{"trailing slash stripped by caller", "https://github.com/org/tool", "tool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, packageNameFromURL(tt.url))
		})
	}
}

// ---------------------------------------------------------------------------
// resolveGitURL
// ---------------------------------------------------------------------------

func TestResolveGitURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "github shorthand",
			input: "github:user/repo",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "full https URL",
			input: "https://github.com/user/repo",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "ssh URL",
			input: "git@github.com:user/repo",
			want:  "git@github.com:user/repo",
		},
		{
			name:  "catalog name superpowers",
			input: "superpowers",
			want:  "https://github.com/obra/superpowers",
		},
		{
			name:    "unknown catalog name",
			input:   "nonexistent-pkg-xyz",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveGitURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// EnsureGitignore
// ---------------------------------------------------------------------------

func TestEnsureGitignore_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	err := EnsureGitignore(dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, ".summon/local/")
	assert.Contains(t, s, ".summon/project/store/")
	assert.Contains(t, s, ".summon/project/platforms/")
}

func TestEnsureGitignore_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644))

	err := EnsureGitignore(dir)
	require.NoError(t, err)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, "node_modules/")
	assert.Contains(t, s, ".summon/local/")
	assert.Contains(t, s, ".summon/project/store/")
	assert.Contains(t, s, ".summon/project/platforms/")
}

func TestEnsureGitignore_NoDuplicates(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, EnsureGitignore(dir))
	require.NoError(t, EnsureGitignore(dir))

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	s := string(content)
	assert.Equal(t, 1, strings.Count(s, ".summon/local/"))
	assert.Equal(t, 1, strings.Count(s, ".summon/project/store/"))
	assert.Equal(t, 1, strings.Count(s, ".summon/project/platforms/"))
}

func TestEnsureGitignore_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")
	// Existing file with no trailing newline.
	require.NoError(t, os.WriteFile(gitignorePath, []byte("node_modules/"), 0o644))

	err := EnsureGitignore(dir)
	require.NoError(t, err)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	lines := strings.Split(string(content), "\n")
	// The first entry should be on a new line, not concatenated with the
	// previous content.
	assert.Equal(t, "node_modules/", lines[0])
	assert.Contains(t, string(content), "\n.summon/local/\n")
}

// ---------------------------------------------------------------------------
// filterCompatible
// ---------------------------------------------------------------------------

func TestFilterCompatible_MatchingPlatforms(t *testing.T) {
	adapters := platform.AllAdapters("")
	// Request only the first adapter's name.
	manifestPlatforms := []string{adapters[0].Name()}

	result := filterCompatible(manifestPlatforms, adapters)
	require.Len(t, result, 1)
	assert.Equal(t, adapters[0].Name(), result[0].Name())
}

func TestFilterCompatible_NoMatch(t *testing.T) {
	adapters := platform.AllAdapters("")
	result := filterCompatible([]string{"nonexistent-platform"}, adapters)
	assert.Empty(t, result)
}

func TestFilterCompatible_EmptyManifestPlatforms(t *testing.T) {
	adapters := platform.AllAdapters("")
	// An empty manifest platforms list means "compatible with all".
	result := filterCompatible(nil, adapters)
	assert.Equal(t, len(adapters), len(result))

	result = filterCompatible([]string{}, adapters)
	assert.Equal(t, len(adapters), len(result))
}

func TestFilterCompatible_EmptyActive(t *testing.T) {
	result := filterCompatible([]string{"claude"}, nil)
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// getPlatformNames
// ---------------------------------------------------------------------------

func TestGetPlatformNames(t *testing.T) {
	adapters := platform.AllAdapters("")
	names := getPlatformNames(adapters)
	require.Len(t, names, len(adapters))
	for i, a := range adapters {
		assert.Equal(t, a.Name(), names[i])
	}
}

func TestGetPlatformNames_Empty(t *testing.T) {
	names := getPlatformNames(nil)
	assert.Nil(t, names)
}

// ---------------------------------------------------------------------------
// reportDependencies
// ---------------------------------------------------------------------------

func TestReportDependencies_NoDeps(t *testing.T) {
	m := &manifest.Manifest{Name: "test", Version: "1.0.0", Description: "d"}
	reg := registry.New()

	// Should not panic and produce no output on stderr.
	old := os.Stderr
	oldWriter := Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	Stderr = w
	reportDependencies(m, reg)
	w.Close()
	os.Stderr = old
	Stderr = oldWriter

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Equal(t, 0, n, "expected no stderr output when there are no deps")
}

func TestReportDependencies_AllSatisfied(t *testing.T) {
	m := &manifest.Manifest{
		Name: "test", Version: "1.0.0", Description: "d",
		Dependencies: map[string]string{"dep-a": ">=1.0.0"},
	}
	reg := registry.New()
	reg.Add("dep-a", registry.Entry{Version: "1.0.0"})

	old := os.Stderr
	oldWriter := Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	Stderr = w
	reportDependencies(m, reg)
	w.Close()
	os.Stderr = old
	Stderr = oldWriter

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Equal(t, 0, n, "expected no warning when all deps are satisfied")
}

func TestReportDependencies_MissingDeps(t *testing.T) {
	m := &manifest.Manifest{
		Name: "test", Version: "1.0.0", Description: "d",
		Dependencies: map[string]string{"missing-dep": ">=1.0.0"},
	}
	reg := registry.New()

	old := os.Stderr
	oldWriter := Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	Stderr = w
	reportDependencies(m, reg)
	w.Close()
	os.Stderr = old
	Stderr = oldWriter

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	assert.Contains(t, output, "missing dependencies")
	assert.Contains(t, output, "missing-dep")
}

// ---------------------------------------------------------------------------
// enablePlugins
// ---------------------------------------------------------------------------

func TestEnablePlugins_WritesEnabledPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	globalDir := filepath.Join(tmpDir, "global-settings")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "test-plugin"), 0o755))
	require.NoError(t, os.MkdirAll(globalDir, 0o755))

	paths := Paths{
		StoreDir:        storeDir,
		RegistryPath:    filepath.Join(tmpDir, "registry.yaml"),
		PlatformsDir:    filepath.Join(tmpDir, "platforms"),
		Scope:           platform.ScopeLocal,
		MarketplaceName: "summon-local",
	}
	adapters := platform.AllAdapters(tmpDir, platform.WithGlobalSettingsDir(globalDir))

	enablePlugins("test-plugin", paths, adapters)

	// Verify each adapter activated the plugin in workspace settings
	for _, a := range adapters {
		settingsPath := a.SettingsPath(platform.ScopeLocal)
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err, "settings file should exist for %s", a.Name())

		var settings map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &settings))

		switch a.Name() {
		case "claude":
			ep, ok := settings["enabledPlugins"].(map[string]interface{})
			require.True(t, ok, "enabledPlugins should be present for claude")
			assert.Equal(t, true, ep["test-plugin@summon-local"])
		case "copilot":
			pl, ok := settings["chat.pluginLocations"].(map[string]interface{})
			require.True(t, ok, "chat.pluginLocations should be present for copilot workspace")
			expectedPath := canonicalPluginPath(storeDir, "test-plugin")
			assert.Equal(t, true, pl[expectedPath])

			// Also verify user-level settings were written
			globalData, err := os.ReadFile(filepath.Join(globalDir, "settings.json"))
			require.NoError(t, err, "user-level settings should exist for copilot")
			var gs map[string]interface{}
			require.NoError(t, json.Unmarshal(globalData, &gs))
			gpl, ok := gs["chat.pluginLocations"].(map[string]interface{})
			require.True(t, ok, "chat.pluginLocations should be present in user-level settings")
			assert.Equal(t, true, gpl[expectedPath])
		}
	}
}

// ---------------------------------------------------------------------------
// expandHookVariables
// ---------------------------------------------------------------------------

func TestExpandHookVariables_ReplacesVariable(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	hooksJSON := `{
  "hooks": {
    "SessionStart": [{
      "hooks": [{
        "type": "command",
        "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd\" session-start"
      }]
    }]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(hooksJSON), 0o644))

	err := expandHookVariables(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	require.NoError(t, err)
	absDir, _ := filepath.Abs(dir)
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}
	assert.Contains(t, string(data), absDir+"/hooks/run-hook.cmd")
	assert.NotContains(t, string(data), "${CLAUDE_PLUGIN_ROOT}")
}

func TestExpandHookVariables_NoHooksJSON(t *testing.T) {
	dir := t.TempDir()
	err := expandHookVariables(dir)
	assert.NoError(t, err)
}

func TestExpandHookVariables_NoVariable(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	hooksJSON := `{"hooks": {}}`
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(hooksJSON), 0o644))

	err := expandHookVariables(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	require.NoError(t, err)
	assert.Equal(t, hooksJSON, string(data))
}

func TestExpandHookVariables_ResolvesSymlinks(t *testing.T) {
	// Simulate a local install where the store entry is a symlink
	realDir := t.TempDir()
	hooksDir := filepath.Join(realDir, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	hooksJSON := `"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd"`
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(hooksJSON), 0o644))

	linkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realDir, linkDir))

	err := expandHookVariables(linkDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	require.NoError(t, err)

	// Should contain the resolved (real) path, not the symlink path
	resolvedDir, _ := filepath.EvalSymlinks(realDir)
	assert.Contains(t, string(data), resolvedDir)
	assert.NotContains(t, string(data), "${CLAUDE_PLUGIN_ROOT}")
}

func TestExpandHookVariables_RootLevelHooksJSON(t *testing.T) {
	dir := t.TempDir()

	hooksJSON := `{
  "hooks": {
    "UserPromptSubmit": [{
      "hooks": [{
        "type": "command",
        "command": "python3 ${CLAUDE_PLUGIN_ROOT}/scripts/hooks/context-hook.py"
      }]
    }]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(hooksJSON), 0o644))

	err := expandHookVariables(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
	require.NoError(t, err)
	absDir, _ := filepath.Abs(dir)
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}
	assert.Contains(t, string(data), absDir+"/scripts/hooks/context-hook.py")
	assert.NotContains(t, string(data), "${CLAUDE_PLUGIN_ROOT}")
}

func TestExpandHookVariables_BothLocations(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	subHooksJSON := `"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd"`
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(subHooksJSON), 0o644))

	rootHooksJSON := `"${CLAUDE_PLUGIN_ROOT}/scripts/context-hook.py"`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(rootHooksJSON), 0o644))

	err := expandHookVariables(dir)
	require.NoError(t, err)

	absDir, _ := filepath.Abs(dir)
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}

	subData, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(subData), absDir+"/hooks/run-hook.cmd")
	assert.NotContains(t, string(subData), "${CLAUDE_PLUGIN_ROOT}")

	rootData, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(rootData), absDir+"/scripts/context-hook.py")
	assert.NotContains(t, string(rootData), "${CLAUDE_PLUGIN_ROOT}")
}

// ---------------------------------------------------------------------------

func TestGenerateMarketplaces_CreatesCorrectStructure(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o755))

	paths := Paths{
		StoreDir:        storeDir,
		RegistryPath:    filepath.Join(tmpDir, "registry.yaml"),
		PlatformsDir:    filepath.Join(tmpDir, "platforms"),
		Scope:           platform.ScopeLocal,
		MarketplaceName: "summon-local",
	}

	reg := registry.New()
	// Create a package in the store with a minimal manifest
	pkgDir := filepath.Join(storeDir, "my-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: my-pkg\nversion: \"1.0.0\"\ndescription: test\n"),
		0o644,
	))
	reg.Add("my-pkg", registry.Entry{Version: "1.0.0"})

	err := generateMarketplaces(paths, reg)
	require.NoError(t, err)

	// marketplace.json must be at .claude-plugin/marketplace.json, NOT at the root
	for _, pname := range []string{"claude", "copilot"} {
		platformDir := filepath.Join(paths.PlatformsDir, pname)

		// Correct location
		correctPath := filepath.Join(platformDir, ".claude-plugin", "marketplace.json")
		assert.FileExists(t, correctPath, "%s marketplace.json should be inside .claude-plugin/", pname)

		// Wrong location must NOT exist
		wrongPath := filepath.Join(platformDir, "marketplace.json")
		assert.NoFileExists(t, wrongPath, "%s marketplace.json must NOT be at root", pname)

		// Symlink must exist
		linkPath := filepath.Join(platformDir, "plugins", "my-pkg")
		fi, err := os.Lstat(linkPath)
		require.NoError(t, err, "symlink should exist for %s", pname)
		assert.True(t, fi.Mode()&os.ModeSymlink != 0, "expected symlink for %s", pname)

		// Source path in marketplace.json must use ./ prefix
		data, err := os.ReadFile(correctPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"source": "./plugins/my-pkg"`,
			"%s source path must use ./ prefix", pname)
		assert.NotContains(t, string(data), `"source": "../`,
			"%s source path must not use ../ traversal", pname)
	}
}

// ---------------------------------------------------------------------------
// T007: scope is persisted in registry.yaml after Install
// ---------------------------------------------------------------------------

func TestInstall_PersistsRegistryScope(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "scope-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: scope-pkg\nversion: \"1.0.0\"\ndescription: \"scope test\"\n"),
		0o644,
	))

	// Only test project-local scopes to avoid writing to the real home directory.
	for _, tc := range []struct {
		scope     platform.Scope
		wantScope string
	}{
		{platform.ScopeLocal, "local"},
		{platform.ScopeProject, "project"},
	} {
		t.Run(tc.wantScope, func(t *testing.T) {
			err := Install(Options{
				Path:       pkgDir,
				Force:      true,
				Scope:      tc.scope,
				ProjectDir: projectDir,
			})
			require.NoError(t, err)

			paths := ResolvePaths(tc.scope, projectDir)
			reg, err := registry.Load(paths.RegistryPath)
			require.NoError(t, err)
			assert.Equal(t, tc.wantScope, reg.Scope,
				"registry.yaml for %s scope should have scope=%s", tc.wantScope, tc.wantScope)
		})
	}
}

// ---------------------------------------------------------------------------
// T023: CopilotAdapter materialization wiring
// ---------------------------------------------------------------------------

// TestMaterializeComponents_ProjectScope verifies that MaterializeComponents
// creates workspace symlinks for skills and agents at project scope.
func TestMaterializeComponents_ProjectScope(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "mat-pkg")
	skillsDir := filepath.Join(pkgDir, "skills")
	agentsDir := filepath.Join(pkgDir, "agents")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: mat-pkg\nversion: \"1.0.0\"\ndescription: \"materialize test\"\ncomponents:\n  skills: skills/\n  agents: agents/\n"),
		0o644,
	))

	m, err := manifest.Load(pkgDir)
	require.NoError(t, err)

	a := &platform.CopilotAdapter{ProjectDir: projectDir}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, platform.ScopeProject))

	// skills and agents should be symlinked into .github/
	_, err = os.Lstat(filepath.Join(projectDir, ".github", "skills", "mat-pkg"))
	require.NoError(t, err, ".github/skills/mat-pkg should exist (symlink)")
	_, err = os.Lstat(filepath.Join(projectDir, ".github", "agents", "mat-pkg"))
	require.NoError(t, err, ".github/agents/mat-pkg should exist (symlink)")
}

// TestMaterializeComponents_LocalScope verifies skills/agents go to .claude/
// at local scope.
func TestMaterializeComponents_LocalScope(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "mat-local-pkg")
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "skills"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: mat-local-pkg\nversion: \"1.0.0\"\ndescription: d\ncomponents:\n  skills: skills/\n"),
		0o644,
	))

	m, err := manifest.Load(pkgDir)
	require.NoError(t, err)

	a := &platform.CopilotAdapter{ProjectDir: projectDir}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, platform.ScopeLocal))

	_, err = os.Lstat(filepath.Join(projectDir, ".claude", "skills", "mat-local-pkg"))
	require.NoError(t, err, ".claude/skills/mat-local-pkg should exist (symlink)")
}

// TestRemoveMaterialized_ProjectScope verifies workspace links are cleaned up.
func TestRemoveMaterialized_ProjectScope(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "rm-mat-pkg")
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "skills"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: rm-mat-pkg\nversion: \"1.0.0\"\ndescription: d\ncomponents:\n  skills: skills/\n"),
		0o644,
	))

	m, err := manifest.Load(pkgDir)
	require.NoError(t, err)

	a := &platform.CopilotAdapter{ProjectDir: projectDir}
	require.NoError(t, a.MaterializeComponents(pkgDir, m, platform.ScopeProject))
	skillsPath := filepath.Join(projectDir, ".github", "skills", "rm-mat-pkg")
	_, err = os.Lstat(skillsPath)
	require.NoError(t, err, "symlink should exist after materialization")

	require.NoError(t, a.RemoveMaterialized("rm-mat-pkg", m, platform.ScopeProject))
	_, err = os.Lstat(skillsPath)
	assert.True(t, os.IsNotExist(err), ".github/skills/rm-mat-pkg should be gone after removal")
}

// ---------------------------------------------------------------------------
// T018: Copilot exact-scope failure diagnostics
// ---------------------------------------------------------------------------

// TestMaterializeComponents_HooksAtProjectScopeReturnsError verifies the
// diagnostic error message when hooks are requested at project scope.
func TestMaterializeComponents_HooksAtProjectScopeReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "hooks-pkg")
	hooksDir := filepath.Join(pkgDir, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: hooks-pkg\nversion: \"1.0.0\"\ndescription: d\ncomponents:\n  hooks: hooks/hooks.json\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte("{}"), 0o644))

	m, err := manifest.Load(pkgDir)
	require.NoError(t, err)

	a := &platform.CopilotAdapter{ProjectDir: projectDir}
	err = a.MaterializeComponents(pkgDir, m, platform.ScopeProject)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hooks")
	assert.Contains(t, err.Error(), "--scope user")
}

// TestMaterializeComponents_MCPAtLocalScopeReturnsError verifies the
// diagnostic error message when MCP is requested at local scope.
func TestMaterializeComponents_MCPAtLocalScopeReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "mcp-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: mcp-pkg\nversion: \"1.0.0\"\ndescription: d\ncomponents:\n  mcp: mcp.json\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "mcp.json"), []byte("{}"), 0o644))

	m, err := manifest.Load(pkgDir)
	require.NoError(t, err)

	a := &platform.CopilotAdapter{ProjectDir: projectDir}
	err = a.MaterializeComponents(pkgDir, m, platform.ScopeLocal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp")
}

// ---------------------------------------------------------------------------
// Installer script contract checks (004-installer-script)
// ---------------------------------------------------------------------------

func TestInstallerScripts_ExistWithExpectedHeaders(t *testing.T) {
	root := filepath.Join("..", "..")

	shPath := filepath.Join(root, "scripts", "install.sh")
	psPath := filepath.Join(root, "scripts", "install.ps1")

	shData, err := os.ReadFile(shPath)
	require.NoError(t, err, "scripts/install.sh should exist")
	psData, err := os.ReadFile(psPath)
	require.NoError(t, err, "scripts/install.ps1 should exist")

	assert.Contains(t, string(shData), "#!/usr/bin/env sh")
	assert.Contains(t, string(psData), "$ErrorActionPreference = \"Stop\"")
}

func TestInstallerScripts_ContainFailureCategories(t *testing.T) {
	root := filepath.Join("..", "..")
	shPath := filepath.Join(root, "scripts", "install.sh")
	psPath := filepath.Join(root, "scripts", "install.ps1")

	shData, err := os.ReadFile(shPath)
	require.NoError(t, err)
	psData, err := os.ReadFile(psPath)
	require.NoError(t, err)

	sh := string(shData)
	ps := string(psData)

	for _, category := range []string{"platform", "download", "checksum", "permission"} {
		assert.Contains(t, sh, "fail "+category)
		assert.Contains(t, ps, "Fail-Installer \""+category+"\"")
	}
}

func TestInstallerScripts_ContainRequiredInputs(t *testing.T) {
	root := filepath.Join("..", "..")
	shPath := filepath.Join(root, "scripts", "install.sh")
	psPath := filepath.Join(root, "scripts", "install.ps1")

	shData, err := os.ReadFile(shPath)
	require.NoError(t, err)
	psData, err := os.ReadFile(psPath)
	require.NoError(t, err)

	sh := string(shData)
	ps := string(psData)

	requiredVars := []string{
		"SUMMON_VERSION",
		"SUMMON_INSTALL_PATH",
		"SUMMON_NO_MODIFY_PATH",
		"SUMMON_NONINTERACTIVE",
		"SUMMON_DOWNLOAD_URL",
		"SUMMON_CHECKSUM_URL",
	}

	for _, v := range requiredVars {
		assert.Contains(t, sh, v)
		assert.Contains(t, ps, v)
	}
}

func runShellInstaller(t *testing.T, env []string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell installer tests only run on unix-like systems")
	}
	root := filepath.Join("..", "..")
	shPath, err := filepath.Abs(filepath.Join(root, "scripts", "install.sh"))
	require.NoError(t, err)
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", shPath)
	cmd.Dir = workDir
	env = append(env, "SUMMON_NONINTERACTIVE=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallerScript_MissingDownloadToolFails(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	env := append(os.Environ(),
		"SUMMON_TEST_DISABLE_DOWNLOAD_TOOL=1",
		"SUMMON_DOWNLOAD_URL=https://example.com/summon",
		"SUMMON_CHECKSUM=deadbeef",
		"SUMMON_INSTALL_PATH="+filepath.Join(t.TempDir(), "bin", "summon"),
		"SUMMON_NO_MODIFY_PATH=1",
	)
	out, err := runShellInstaller(t, env)
	require.Error(t, err)
	assert.Contains(t, out, "ERROR[download]")
}

func TestInstallerScript_MissingChecksumToolFails(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	source := filepath.Join(t.TempDir(), "summon-source")
	require.NoError(t, os.WriteFile(source, []byte("#!/usr/bin/env sh\necho x\n"), 0o755))
	env := append(os.Environ(),
		"SUMMON_TEST_DISABLE_HASH_TOOL=1",
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+source,
		"SUMMON_CHECKSUM=deadbeef",
		"SUMMON_INSTALL_PATH="+filepath.Join(t.TempDir(), "bin", "summon"),
		"SUMMON_NO_MODIFY_PATH=1",
	)
	out, err := runShellInstaller(t, env)
	require.Error(t, err)
	assert.Contains(t, out, "ERROR[checksum]")
}

func TestInstallerScript_UnsupportedPlatformFails(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	env := append(os.Environ(),
		"SUMMON_TEST_OS=plan9",
		"SUMMON_NO_MODIFY_PATH=1",
	)
	out, err := runShellInstaller(t, env)
	require.Error(t, err)
	assert.Contains(t, out, "ERROR[platform]")
}

func TestInstallerScript_UnwritableTargetFails(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	env := append(os.Environ(),
		"SUMMON_INSTALL_PATH=/dev/null/summon",
		"SUMMON_NO_MODIFY_PATH=1",
	)
	out, err := runShellInstaller(t, env)
	require.Error(t, err)
	assert.Contains(t, out, "ERROR[permission]")
}

func TestInstallerScripts_ContainNoPromptCommands(t *testing.T) {
	root := filepath.Join("..", "..")
	shPath := filepath.Join(root, "scripts", "install.sh")
	psPath := filepath.Join(root, "scripts", "install.ps1")

	shData, err := os.ReadFile(shPath)
	require.NoError(t, err)
	psData, err := os.ReadFile(psPath)
	require.NoError(t, err)

	sh := string(shData)
	ps := string(psData)

	// Prompts must not use `read -p` (non-portable); `read -r` via read_input() is fine
	assert.NotContains(t, sh, "read -p")
	// Both scripts must gate interactive prompts behind SUMMON_NONINTERACTIVE
	assert.Contains(t, sh, "SUMMON_NONINTERACTIVE")
	assert.Contains(t, ps, "SUMMON_NONINTERACTIVE")
}

func TestInstallerScripts_ContainPathOptOutAndFallback(t *testing.T) {
	root := filepath.Join("..", "..")
	shPath := filepath.Join(root, "scripts", "install.sh")
	psPath := filepath.Join(root, "scripts", "install.ps1")

	shData, err := os.ReadFile(shPath)
	require.NoError(t, err)
	psData, err := os.ReadFile(psPath)
	require.NoError(t, err)

	sh := string(shData)
	ps := string(psData)

	assert.Contains(t, sh, "SUMMON_NO_MODIFY_PATH")
	assert.Contains(t, ps, "SUMMON_NO_MODIFY_PATH")
	assert.Contains(t, sh, "Run manually:")
	assert.Contains(t, ps, "Run manually:")
}

// ---------------------------------------------------------------------------
// plugin.json fallback tests (004-plugin-json-fallback)
// ---------------------------------------------------------------------------

func TestInstall_PluginJSONFallback(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "pj-only-pkg")
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, ".claude-plugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "skills"), 0o755))

	pj := map[string]any{
		"name": "pj-only-pkg", "version": "1.0.0", "description": "plugin.json only",
	}
	pjData, _ := json.Marshal(pj)
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, ".claude-plugin", "plugin.json"), pjData, 0o644))

	err := Install(Options{
		Path:       pkgDir,
		Force:      true,
		Scope:      platform.ScopeLocal,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)

	paths := ResolvePaths(platform.ScopeLocal, projectDir)
	reg, err := registry.Load(paths.RegistryPath)
	require.NoError(t, err)
	entry, ok := reg.Get("pj-only-pkg")
	require.True(t, ok, "pj-only-pkg should be in registry")
	assert.Equal(t, "1.0.0", entry.Version)
}

func TestInstall_SkipGeneratePluginJSON(t *testing.T) {
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "custom-pj-pkg")
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, ".claude-plugin"), 0o755))

	// Write summon.yaml so the normal flow is used
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte("name: custom-pj-pkg\nversion: \"1.0.0\"\ndescription: test\n"),
		0o644,
	))

	// Write a custom plugin.json with extra fields
	customPJ := `{"name":"custom-pj-pkg","version":"1.0.0","description":"test","keywords":["custom"]}`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, ".claude-plugin", "plugin.json"), []byte(customPJ), 0o644))

	err := Install(Options{
		Path:       pkgDir,
		Force:      true,
		Scope:      platform.ScopeLocal,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)

	// Verify the custom plugin.json was preserved (not overwritten by GeneratePluginJSON)
	paths := ResolvePaths(platform.ScopeLocal, projectDir)
	storePJ := filepath.Join(paths.StoreDir, "custom-pj-pkg", ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(storePJ)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"keywords"`, "custom plugin.json should be preserved")
}

func TestInstall_MarketplaceExtraction(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "marketplace-repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	// Create two plugin subdirs with plugin.json
	for _, name := range []string{"mp-plugin-a", "mp-plugin-b"} {
		sub := filepath.Join(repoDir, name)
		require.NoError(t, os.MkdirAll(filepath.Join(sub, ".claude-plugin"), 0o755))
		pj, _ := json.Marshal(map[string]any{
			"name": name, "version": "1.0.0", "description": name + " desc",
		})
		require.NoError(t, os.WriteFile(filepath.Join(sub, ".claude-plugin", "plugin.json"), pj, 0o644))
	}

	// Create marketplace.json at repo root
	mpDir := filepath.Join(repoDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(mpDir, 0o755))
	mp, _ := json.Marshal(map[string]any{
		"name": "test-marketplace",
		"plugins": []map[string]any{
			{"name": "mp-plugin-a", "source": "./mp-plugin-a"},
			{"name": "mp-plugin-b", "source": "./mp-plugin-b"},
		},
	})
	require.NoError(t, os.WriteFile(filepath.Join(mpDir, "marketplace.json"), mp, 0o644))

	err := Install(Options{
		Path:       repoDir,
		Force:      true,
		Scope:      platform.ScopeLocal,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)

	paths := ResolvePaths(platform.ScopeLocal, projectDir)
	reg, err := registry.Load(paths.RegistryPath)
	require.NoError(t, err)

	entryA, okA := reg.Get("mp-plugin-a")
	entryB, okB := reg.Get("mp-plugin-b")
	require.True(t, okA, "mp-plugin-a should be in registry")
	require.True(t, okB, "mp-plugin-b should be in registry")
	assert.Equal(t, "1.0.0", entryA.Version)
	assert.Equal(t, "1.0.0", entryB.Version)
}

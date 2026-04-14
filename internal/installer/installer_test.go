package installer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ResolvePaths
// ---------------------------------------------------------------------------

func TestResolvePaths_Local(t *testing.T) {
	proj := filepath.Join("C:", "proj")
	if runtime.GOOS != "windows" {
		proj = "/proj"
	}
	p := ResolvePaths(platform.ScopeLocal, proj)
	assert.Equal(t, filepath.Join(proj, ".summon", "local", "store"), p.StoreDir)
	assert.Equal(t, filepath.Join(proj, ".summon", "local", "registry.yaml"), p.RegistryPath)
	assert.Equal(t, platform.ScopeLocal, p.Scope)
}

func TestResolvePaths_Project(t *testing.T) {
	proj := filepath.Join("C:", "proj")
	if runtime.GOOS != "windows" {
		proj = "/proj"
	}
	p := ResolvePaths(platform.ScopeProject, proj)
	assert.Equal(t, filepath.Join(proj, ".summon", "project", "store"), p.StoreDir)
	assert.Equal(t, filepath.Join(proj, ".summon", "project", "registry.yaml"), p.RegistryPath)
	assert.Equal(t, platform.ScopeProject, p.Scope)
}

func TestResolvePaths_User(t *testing.T) {
	p := ResolvePaths(platform.ScopeUser, "/proj")
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".summon", "user", "store"), p.StoreDir)
	assert.Equal(t, filepath.Join(home, ".summon", "user", "registry.yaml"), p.RegistryPath)
	assert.Equal(t, platform.ScopeUser, p.Scope)
}

// ---------------------------------------------------------------------------
// MakeScopedTempDir
// ---------------------------------------------------------------------------

func TestMakeScopedTempDir_CreatesDir(t *testing.T) {
	base := t.TempDir()
	paths := Paths{StoreDir: filepath.Join(base, "store")}
	tmpDir, err := MakeScopedTempDir(paths, "test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	assert.DirExists(t, tmpDir)
}

// ---------------------------------------------------------------------------
// packageNameFromURL
// ---------------------------------------------------------------------------

func TestPackageNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/my-plugin", "my-plugin"},
		{"https://github.com/user/my-plugin.git", "my-plugin"},
		{"git@github.com:user/repo.git", "repo"},
		{"simple-name", "simple-name"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, packageNameFromURL(tt.url))
		})
	}
}

// ---------------------------------------------------------------------------
// resolveGitURL
// ---------------------------------------------------------------------------

func TestResolveGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github:user/repo", "https://github.com/user/repo"},
		{"https://github.com/user/repo", "https://github.com/user/repo"},
		{"git@github.com:user/repo.git", "git@github.com:user/repo.git"},
		{"file:///tmp/repo", "file:///tmp/repo"},
		{"my-plugin", "https://github.com/ai-summon/my-plugin"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := resolveGitURL(tt.input)
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
	require.NoError(t, EnsureGitignore(dir))
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".summon/local/")
	assert.Contains(t, string(data), ".summon/project/store/")
}

func TestEnsureGitignore_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, EnsureGitignore(dir))
	require.NoError(t, EnsureGitignore(dir))
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	assert.Equal(t, 1, strings.Count(string(data), ".summon/local/"))
}

// ---------------------------------------------------------------------------
// getPlatformNames
// ---------------------------------------------------------------------------

func TestGetPlatformNames(t *testing.T) {
	adapters := platform.AllAdapters("/proj")
	names := getPlatformNames(adapters)
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "copilot")
}

func TestGetPlatformNames_Empty(t *testing.T) {
	names := getPlatformNames(nil)
	assert.Nil(t, names)
}

// ---------------------------------------------------------------------------
// expandHookVariables
// ---------------------------------------------------------------------------

func TestExpandHookVariables_ReplacesVariable(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	content := `{"cmd": "PLUGIN_ROOT/run.sh"}`
	content = strings.ReplaceAll(content, "PLUGIN_ROOT", "${CLAUDE_PLUGIN_ROOT}")
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "hooks.json"),
		[]byte(content),
		0o644,
	))

	require.NoError(t, expandHookVariables(dir))

	data, _ := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	assert.NotContains(t, string(data), "${CLAUDE"+"_PLUGIN_ROOT}")
	// Verify the variable was replaced with some real path containing the dir name.
	// On Windows, short path names (e.g., RUNNER~1) may differ from filepath.Abs,
	// so just check the variable substitution happened.
	assert.Contains(t, string(data), "run.sh")
}

func TestExpandHookVariables_NoHooksJSON(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, expandHookVariables(dir))
}

// ---------------------------------------------------------------------------
// scopeFromLegacy
// ---------------------------------------------------------------------------

func TestScopeFromLegacy(t *testing.T) {
	// When scope is explicitly set, global flag is ignored
	assert.Equal(t, platform.ScopeLocal, scopeFromLegacy(true, platform.ScopeLocal))
	assert.Equal(t, platform.ScopeProject, scopeFromLegacy(false, platform.ScopeProject))
	assert.Equal(t, platform.ScopeUser, scopeFromLegacy(false, platform.ScopeUser))
	// When scope is not a valid enum value (e.g. 99), falls back to global flag
	assert.Equal(t, platform.ScopeUser, scopeFromLegacy(true, 99))
	assert.Equal(t, platform.ScopeLocal, scopeFromLegacy(false, 99))
}

// ---------------------------------------------------------------------------
// fileExists
// ---------------------------------------------------------------------------

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	assert.False(t, fileExists(f))
	require.NoError(t, os.WriteFile(f, []byte("hi"), 0o644))
	assert.True(t, fileExists(f))
	assert.False(t, fileExists(dir)) // directory, not file
}

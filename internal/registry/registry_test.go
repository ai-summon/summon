package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	r := New()
	assert.Equal(t, "0.1.0", r.SummonVersion)
	assert.NotNil(t, r.Packages)
	assert.Empty(t, r.Packages)
}

func TestAddAndGet(t *testing.T) {
	r := New()
	r.Add("test-pkg", Entry{
		Version: "1.0.0",
		Source:  Source{Type: "github", URL: "https://github.com/user/test-pkg"},
	})
	entry, ok := r.Get("test-pkg")
	assert.True(t, ok)
	assert.Equal(t, "1.0.0", entry.Version)
	assert.Equal(t, "github", entry.Source.Type)
	assert.NotEmpty(t, entry.InstalledAt)
}

func TestHas(t *testing.T) {
	r := New()
	assert.False(t, r.Has("missing"))
	r.Add("test-pkg", Entry{Version: "1.0.0"})
	assert.True(t, r.Has("test-pkg"))
}

func TestRemove(t *testing.T) {
	r := New()
	r.Add("test-pkg", Entry{Version: "1.0.0"})
	removed := r.Remove("test-pkg")
	assert.True(t, removed)
	assert.False(t, r.Has("test-pkg"))
}

func TestRemove_NotFound(t *testing.T) {
	r := New()
	removed := r.Remove("missing")
	assert.False(t, removed)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	r := New()
	r.Add("pkg-a", Entry{
		Version:   "2.0.0",
		Source:    Source{Type: "local", URL: "/tmp/pkg-a"},
		Platforms: []string{"claude"},
	})
	err := r.Save(path)
	require.NoError(t, err)
	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", loaded.SummonVersion)
	entry, ok := loaded.Get("pkg-a")
	assert.True(t, ok)
	assert.Equal(t, "2.0.0", entry.Version)
	assert.Equal(t, "local", entry.Source.Type)
	assert.Equal(t, []string{"claude"}, entry.Platforms)
}

func TestLoad_NonExistent(t *testing.T) {
	r, err := Load("/nonexistent/path/registry.yaml")
	require.NoError(t, err)
	assert.NotNil(t, r.Packages)
	assert.Empty(t, r.Packages)
}

func TestSave_CreatesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "registry.yaml")
	r := New()
	err := r.Save(path)
	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	require.NoError(t, os.WriteFile(path, []byte(":\n\t[bad yaml"), 0o644))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing registry")
}

func TestLoad_NilPackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	require.NoError(t, os.WriteFile(path, []byte("summon_version: \"0.1.0\"\n"), 0o644))
	r, err := Load(path)
	require.NoError(t, err)
	assert.NotNil(t, r.Packages)
	assert.Empty(t, r.Packages)
}

func TestAdd_OverwritesExisting(t *testing.T) {
	r := New()
	r.Add("pkg", Entry{Version: "1.0.0"})
	r.Add("pkg", Entry{Version: "2.0.0"})
	entry, ok := r.Get("pkg")
	require.True(t, ok)
	assert.Equal(t, "2.0.0", entry.Version)
	assert.Len(t, r.Packages, 1)
}

func TestAdd_SetsInstalledAt(t *testing.T) {
	r := New()
	r.Add("pkg", Entry{Version: "1.0.0"})
	entry, ok := r.Get("pkg")
	require.True(t, ok)
	assert.NotEmpty(t, entry.InstalledAt, "InstalledAt should be set automatically")
}

func TestGet_NotFound(t *testing.T) {
	r := New()
	entry, ok := r.Get("missing")
	assert.False(t, ok)
	assert.Equal(t, Entry{}, entry)
}

func TestRegistry_Scope_EmptyByDefault(t *testing.T) {
	r := New()
	assert.Empty(t, r.Scope)
}

func TestRegistry_Scope_IsPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	r := New()
	r.Scope = "project"
	r.Add("pkg-a", Entry{Version: "1.0.0"})
	require.NoError(t, r.Save(path))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "project", loaded.Scope)
}

func TestRegistry_Scope_RoundtripAllValues(t *testing.T) {
	for _, scope := range []string{"user", "project", "local"} {
		t.Run(scope, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "registry.yaml")
			r := New()
			r.Scope = scope
			require.NoError(t, r.Save(path))
			loaded, err := Load(path)
			require.NoError(t, err)
			assert.Equal(t, scope, loaded.Scope)
		})
	}
}

// ---------------------------------------------------------------------------
// T026: same-package multi-scope behavior
// ---------------------------------------------------------------------------

// TestMultiScope_SamePackageInMultipleScopes verifies that the same package
// can coexist in distinct scope registries without cross-contamination.
func TestMultiScope_SamePackageInMultipleScopes(t *testing.T) {
	localDir := t.TempDir()
	projectDir := t.TempDir()
	userDir := t.TempDir()

	localPath := filepath.Join(localDir, "registry.yaml")
	projectPath := filepath.Join(projectDir, "registry.yaml")
	userPath := filepath.Join(userDir, "registry.yaml")

	entry := func(version string) Entry {
		return Entry{Version: version, Source: Source{Type: "local", URL: "/tmp/pkg"}}
	}

	// Install same package under three different scopes (separate registry files).
	rLocal := New()
	rLocal.Scope = "local"
	rLocal.Add("shared-pkg", entry("1.0.0"))
	require.NoError(t, rLocal.Save(localPath))

	rProject := New()
	rProject.Scope = "project"
	rProject.Add("shared-pkg", entry("2.0.0"))
	require.NoError(t, rProject.Save(projectPath))

	rUser := New()
	rUser.Scope = "user"
	rUser.Add("shared-pkg", entry("3.0.0"))
	require.NoError(t, rUser.Save(userPath))

	// Reload each and verify isolation.
	loadedLocal, err := Load(localPath)
	require.NoError(t, err)
	assert.Equal(t, "local", loadedLocal.Scope)
	e, ok := loadedLocal.Get("shared-pkg")
	require.True(t, ok)
	assert.Equal(t, "1.0.0", e.Version)

	loadedProject, err := Load(projectPath)
	require.NoError(t, err)
	assert.Equal(t, "project", loadedProject.Scope)
	e, ok = loadedProject.Get("shared-pkg")
	require.True(t, ok)
	assert.Equal(t, "2.0.0", e.Version)

	loadedUser, err := Load(userPath)
	require.NoError(t, err)
	assert.Equal(t, "user", loadedUser.Scope)
	e, ok = loadedUser.Get("shared-pkg")
	require.True(t, ok)
	assert.Equal(t, "3.0.0", e.Version)
}

// ---------------------------------------------------------------------------
// T030: removing from one scope does not affect other scopes
// ---------------------------------------------------------------------------

// TestMultiScope_RemoveFromOneScope verifies that removing a package from one
// scope's registry leaves the other scopes' registries untouched.
func TestMultiScope_RemoveFromOneScope(t *testing.T) {
	localDir := t.TempDir()
	projectDir := t.TempDir()

	localPath := filepath.Join(localDir, "registry.yaml")
	projectPath := filepath.Join(projectDir, "registry.yaml")

	entry := Entry{Version: "1.0.0", Source: Source{Type: "local", URL: "/tmp/pkg"}}

	// Populate both registries.
	rLocal := New()
	rLocal.Scope = "local"
	rLocal.Add("shared-pkg", entry)
	require.NoError(t, rLocal.Save(localPath))

	rProject := New()
	rProject.Scope = "project"
	rProject.Add("shared-pkg", entry)
	require.NoError(t, rProject.Save(projectPath))

	// Remove from local scope only.
	rLocal.Remove("shared-pkg")
	require.NoError(t, rLocal.Save(localPath))

	// local: package should be gone
	loadedLocal, err := Load(localPath)
	require.NoError(t, err)
	assert.False(t, loadedLocal.Has("shared-pkg"), "local scope should no longer have shared-pkg")

	// project: package must still be present
	loadedProject, err := Load(projectPath)
	require.NoError(t, err)
	assert.True(t, loadedProject.Has("shared-pkg"), "project scope should still have shared-pkg")
	e, ok := loadedProject.Get("shared-pkg")
	require.True(t, ok)
	assert.Equal(t, "1.0.0", e.Version)
}

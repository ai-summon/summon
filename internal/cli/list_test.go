package cli

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

func TestListCmd_Flags(t *testing.T) {
	flags := listCmd.Flags()
	assert.NotNil(t, flags.Lookup("global"), "list should have --global flag")
	assert.NotNil(t, flags.Lookup("scope"), "list should have --scope flag")
	assert.NotNil(t, flags.Lookup("json"), "list should have --json flag")
}

func TestRunList_DefaultIncludesAllVisibleScopes(t *testing.T) {
	dir := setupProjectDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  local-pkg:
    version: "1.0.0"
    source:
      type: local
      url: /tmp/local-pkg
    platforms: []
`)
	createScopedStorePackage(t, dir, "local", "local-pkg")

	writeScopedRegistryYAML(t, dir, "project", `
summon_version: "0.1.0"
packages:
  project-pkg:
    version: "2.0.0"
    source:
      type: github
      url: https://github.com/org/project-pkg
    platforms: []
`)
	createScopedStorePackage(t, dir, "project", "project-pkg")

	writeScopedRegistryYAML(t, home, "user", `
summon_version: "0.1.0"
packages:
  user-pkg:
    version: "3.0.0"
    source:
      type: github
      url: https://github.com/org/user-pkg
    platforms: []
`)
	createScopedStorePackage(t, home, "user", "user-pkg")

	listGlobal = false
	listScope = ""
	listJSON = true

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 3)

	byName := map[string]listEntry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	assert.Equal(t, "local", byName["local-pkg"].Scope)
	assert.Equal(t, "project", byName["project-pkg"].Scope)
	assert.Equal(t, "user", byName["user-pkg"].Scope)
}

func TestRunList_ScopeFilter_Project(t *testing.T) {
	dir := setupProjectDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  local-only:
    version: "1.0.0"
    source:
      type: local
      url: /tmp/local-only
    platforms: []
`)
	writeScopedRegistryYAML(t, dir, "project", `
summon_version: "0.1.0"
packages:
  project-only:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/project-only
    platforms: []
`)
	writeScopedRegistryYAML(t, home, "user", `
summon_version: "0.1.0"
packages:
  user-only:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/user-only
    platforms: []
`)

	listGlobal = false
	listScope = "project"
	listJSON = true

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "project-only", entries[0].Name)
	assert.Equal(t, "project", entries[0].Scope)
}

func TestExtractRepoPath(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "standard github URL",
			url:  "https://github.com/obra/superpowers",
			want: "obra/superpowers",
		},
		{
			name: "github URL with trailing slash",
			url:  "https://github.com/org/repo/",
			want: "org/repo/",
		},
		{
			name: "non-github URL longer than prefix is still stripped by length",
			url:  "https://gitlab.com/org/repo",
			want: "org/repo",
		},
		{
			name: "exact prefix only (no path after it)",
			url:  "https://github.com/",
			want: "https://github.com/",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractRepoPath(tt.url))
		})
	}
}

func TestRunList_EmptyRegistry(t *testing.T) {
	_ = setupProjectDir(t)

	listGlobal = false
	listScope = "local"
	listJSON = false

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages installed")
}

func TestRunList_TextOutput(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  my-pkg:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/my-pkg
      ref: v1.0.0
      sha: abc123
    platforms: [claude]
`)
	createStorePackage(t, dir, "my-pkg")

	listGlobal = false
	listScope = ""
	listJSON = false

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Package")
	assert.Contains(t, out, "Version")
	assert.Contains(t, out, "Scope")
	assert.Contains(t, out, "Source")
	assert.Contains(t, out, "───")
	assert.Contains(t, out, "my-pkg")
	assert.Contains(t, out, "1.0.0")
	assert.Contains(t, out, "local")
	assert.Contains(t, out, "github:org/my-pkg@v1.0.0")
}

func TestRunList_JSONOutput(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  json-pkg:
    version: "2.0.0"
    source:
      type: github
      url: https://github.com/org/json-pkg
    platforms: [claude]
`)
	createStorePackage(t, dir, "json-pkg")

	listGlobal = false
	listScope = "local"
	listJSON = true

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries), "output should be valid JSON")
	require.Len(t, entries, 1)
	assert.Equal(t, "json-pkg", entries[0].Name)
	assert.Equal(t, "2.0.0", entries[0].Version)
	assert.Equal(t, "local", entries[0].Scope)
}

func TestRunList_BrokenLink(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  broken-pkg:
    version: "0.1.0"
    source:
      type: local
      url: /nonexistent/path
    platforms: []
`)
	storeDir := filepath.Join(dir, ".summon", "local", "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o755))
	require.NoError(t, os.Symlink("/nonexistent/target", filepath.Join(storeDir, "broken-pkg")))

	listGlobal = false
	listScope = ""
	listJSON = false

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "broken-pkg")
	assert.Contains(t, out, "(broken)")
}

func TestRunList_MultiplePackages(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  alpha:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/alpha
    platforms: [claude]
  beta:
    version: "2.0.0"
    source:
      type: local
      url: /some/path
    platforms: [copilot]
`)
	createStorePackage(t, dir, "alpha")
	createStorePackage(t, dir, "beta")

	listGlobal = false
	listScope = "local"
	listJSON = true

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	assert.Len(t, entries, 2)

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	assert.True(t, names["alpha"])
	assert.True(t, names["beta"])
}

func TestRunList_TextSortOrder(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  zeta:
    version: "1.0.0"
    source:
      type: local
      url: /tmp/zeta
    platforms: []
  alpha:
    version: "2.0.0"
    source:
      type: local
      url: /tmp/alpha
    platforms: []
`)
	createStorePackage(t, dir, "zeta")
	createStorePackage(t, dir, "alpha")

	listGlobal = false
	listScope = "local"
	listJSON = false

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})

	alphaIdx := strings.Index(out, "alpha")
	zetaIdx := strings.Index(out, "\nzeta")
	require.Greater(t, alphaIdx, 0, "alpha should appear in output")
	require.Greater(t, zetaIdx, 0, "zeta should appear in output")
	assert.Less(t, alphaIdx, zetaIdx, "alpha should appear before zeta (sorted)")
}

func TestRunList_SourceFormatting_LocalVsGitHub(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  gh-no-ref:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/gh-no-ref
    platforms: []
  local-src:
    version: "0.5.0"
    source:
      type: local
      url: /some/local/path
    platforms: []
`)
	createStorePackage(t, dir, "gh-no-ref")
	createStorePackage(t, dir, "local-src")

	listGlobal = false
	listScope = ""
	listJSON = true

	out := captureStdout(t, func() {
		err := runList(listCmd, nil)
		assert.NoError(t, err)
	})

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))

	byName := map[string]listEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	assert.Equal(t, "https://github.com/org/gh-no-ref", byName["gh-no-ref"].Source)
	assert.Equal(t, "/some/local/path", byName["local-src"].Source)
}

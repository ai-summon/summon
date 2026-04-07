package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateCmd_Flags(t *testing.T) {
	assert.NotNil(t, updateCmd.Flags().Lookup("global"), "update should have --global flag")
	assert.NotNil(t, updateCmd.Flags().Lookup("scope"), "update should have --scope flag")
}

func TestUpdateCmd_ArgsValidator(t *testing.T) {
	assert.NoError(t, updateCmd.Args(updateCmd, []string{}))
	assert.NoError(t, updateCmd.Args(updateCmd, []string{"one"}))
	assert.Error(t, updateCmd.Args(updateCmd, []string{"a", "b"}),
		"update should reject more than 1 positional arg")
}

func TestRunUpdate_EmptyRegistry(t *testing.T) {
	_ = setupProjectDir(t)
	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages to update")
}

func TestRunUpdate_PackageNotInstalled(t *testing.T) {
	_ = setupProjectDir(t)
	updateGlobal = false
	updateScope = ""

	err := runUpdate(updateCmd, []string{"missing-pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"missing-pkg" is not installed`)
}

func TestRunUpdate_LocalPackage(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  local-pkg:
    version: "1.0.0"
    source:
      type: local
      url: /original/path
    platforms: [claude]
`)
	createStorePackage(t, dir, "local-pkg")
	manifest := `name: local-pkg
version: "1.0.0"
description: "A local package"
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".summon", "local", "store", "local-pkg", "summon.yaml"),
		[]byte(manifest), 0o644,
	))
	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"local-pkg"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Regenerated marketplace views for local-pkg")
	assert.Contains(t, out, "Updated 1 package(s)")

	// plugin.json should have been created.
	pluginJSON := filepath.Join(dir, ".summon", "local", "store", "local-pkg", ".claude-plugin", "plugin.json")
	_, err := os.Stat(pluginJSON)
	assert.NoError(t, err, "plugin.json should be generated")
}

func TestRunUpdate_AllPackages_NoneInstalled(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages: {}
`)
	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages to update")
}

func TestRunUpdate_LocalPackageWithPluginJSON(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  plugin-pkg:
    version: "1.0.0"
    source:
      type: local
      url: /original/path
    platforms: [claude]
`)
	createStorePackage(t, dir, "plugin-pkg")

	// Create plugin.json instead of summon.yaml
	pluginDir := filepath.Join(dir, ".summon", "local", "store", "plugin-pkg", ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pluginJSON := `{"name":"plugin-pkg","version":"1.0.0","description":"A plugin.json package"}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginJSON), 0o644))

	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"plugin-pkg"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Regenerated marketplace views for plugin-pkg")
	assert.Contains(t, out, "Updated 1 package(s)")
}

func TestRunUpdate_AmbiguousPackageRequiresScope(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  shared-pkg:
    version: "1.0.0"
    source:
      type: local
      url: /tmp/local-shared
    platforms: []
`)
	writeScopedRegistryYAML(t, dir, "project", `
summon_version: "0.1.0"
packages:
  shared-pkg:
    version: "2.0.0"
    source:
      type: github
      url: https://github.com/org/shared-pkg
    platforms: []
`)

	updateGlobal = false
	updateScope = ""

	err := runUpdate(updateCmd, []string{"shared-pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installed in multiple scopes")
	assert.Contains(t, err.Error(), "--scope")
}

// initGitRepo creates a bare-minimum git repo with one commit and returns its path.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte("name: test-pkg\nversion: \"1.0.0\"\ndescription: test\n"), 0o644))
	add := exec.Command("git", "add", ".")
	add.Dir = dir
	require.NoError(t, add.Run())
	commit := exec.Command("git", "commit", "-m", "init")
	commit.Dir = dir
	out, err := commit.CombinedOutput()
	require.NoError(t, err, string(out))
	return dir
}

// gitSHA returns the current HEAD SHA of the repo at dir.
func gitSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out[:len(out)-1])
}

func TestRunUpdate_GitHubPackagePinnedRef_PreservesRef(t *testing.T) {
	dir := setupProjectDir(t)

	// Create a "remote" repo with a tag
	remote := initGitRepo(t, filepath.Join(t.TempDir(), "remote"))
	tagCmd := exec.Command("git", "tag", "v1.0.0")
	tagCmd.Dir = remote
	require.NoError(t, tagCmd.Run())

	// Clone it into the store path (simulates an installed package)
	storePath := filepath.Join(dir, ".summon", "local", "store", "test-pkg")
	cloneCmd := exec.Command("git", "clone", "--quiet", remote, storePath)
	require.NoError(t, cloneCmd.Run())

	sha := gitSHA(t, storePath)

	// Add a new commit + tag to the remote (simulates upstream releasing v2.0.0)
	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "v2 release")
	commitCmd.Dir = remote
	require.NoError(t, commitCmd.Run())
	tag2Cmd := exec.Command("git", "tag", "v2.0.0")
	tag2Cmd.Dir = remote
	require.NoError(t, tag2Cmd.Run())

	// Write registry with pinned ref to v1.0.0
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  test-pkg:
    version: "1.0.0"
    source:
      type: github
      url: `+remote+`
      ref: v1.0.0
      sha: `+sha+`
    platforms: [claude]
`)

	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"test-pkg"})
		assert.NoError(t, err)
	})
	// Should stay at v1.0.0, not jump to v2.0.0
	_ = out

	// Verify ref is preserved in registry
	reg, err := registry.Load(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	entry, ok := reg.Get("test-pkg")
	require.True(t, ok)
	assert.Equal(t, "v1.0.0", entry.Source.Ref, "pinned ref should be preserved after update")
}

func TestRunUpdate_GitHubPackagePinnedRef_MissingRef_ReturnsError(t *testing.T) {
	dir := setupProjectDir(t)

	// Create a "remote" repo with only v1.0.0
	remote := initGitRepo(t, filepath.Join(t.TempDir(), "remote"))
	tagCmd := exec.Command("git", "tag", "v1.0.0")
	tagCmd.Dir = remote
	require.NoError(t, tagCmd.Run())

	// Clone into store
	storePath := filepath.Join(dir, ".summon", "local", "store", "test-pkg")
	cloneCmd := exec.Command("git", "clone", "--quiet", remote, storePath)
	require.NoError(t, cloneCmd.Run())

	sha := gitSHA(t, storePath)

	// Registry has a ref that doesn't exist in the remote
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  test-pkg:
    version: "1.0.0"
    source:
      type: github
      url: `+remote+`
      ref: v99.0.0
      sha: `+sha+`
    platforms: [claude]
`)

	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"test-pkg"})
		// Per-package errors are logged, not returned — top-level succeeds
		assert.NoError(t, err)
	})
	// The update count should be 0 (failed update is not counted)
	assert.Contains(t, out, "Updated 0 package(s)")

	// Verify registry entry is unchanged (not overwritten)
	reg, err := registry.Load(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	entry, ok := reg.Get("test-pkg")
	require.True(t, ok)
	assert.Equal(t, "v99.0.0", entry.Source.Ref, "ref should be unchanged after failed update")
	assert.Equal(t, sha, entry.Source.SHA, "SHA should be unchanged after failed update")
}

func TestRunUpdate_GitHubPackageBranchRef_PreservesRefAndPulls(t *testing.T) {
	dir := setupProjectDir(t)

	// Create a "remote" repo on the default branch (master/main)
	remote := initGitRepo(t, filepath.Join(t.TempDir(), "remote"))

	// Clone into store
	storePath := filepath.Join(dir, ".summon", "local", "store", "test-pkg")
	cloneCmd := exec.Command("git", "clone", "--quiet", remote, storePath)
	require.NoError(t, cloneCmd.Run())

	// Determine the default branch name
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = storePath
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	branch := string(branchOut[:len(branchOut)-1])

	oldSHA := gitSHA(t, storePath)

	// Add a new commit to the remote (simulates upstream activity)
	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "new-commit")
	commitCmd.Dir = remote
	require.NoError(t, commitCmd.Run())
	newRemoteSHA := gitSHA(t, remote)

	// Write registry with branch ref
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  test-pkg:
    version: "1.0.0"
    source:
      type: github
      url: `+remote+`
      ref: `+branch+`
      sha: `+oldSHA+`
    platforms: [claude]
`)

	updateGlobal = false
	updateScope = ""

	captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"test-pkg"})
		assert.NoError(t, err)
	})

	// Verify ref is preserved as the branch name
	reg, loadErr := registry.Load(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, loadErr)
	entry, ok := reg.Get("test-pkg")
	require.True(t, ok)
	assert.Equal(t, branch, entry.Source.Ref, "branch ref should be preserved after update")
	assert.Equal(t, newRemoteSHA, entry.Source.SHA, "SHA should be updated to latest commit on branch")
}

func TestRunUpdate_GitHubPackageUnpinned_ResolvesLatest(t *testing.T) {
	dir := setupProjectDir(t)

	// Create a "remote" repo with two tags
	remote := initGitRepo(t, filepath.Join(t.TempDir(), "remote"))
	tag1Cmd := exec.Command("git", "tag", "v1.0.0")
	tag1Cmd.Dir = remote
	require.NoError(t, tag1Cmd.Run())

	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "v2 release")
	commitCmd.Dir = remote
	require.NoError(t, commitCmd.Run())
	tag2Cmd := exec.Command("git", "tag", "v2.0.0")
	tag2Cmd.Dir = remote
	require.NoError(t, tag2Cmd.Run())

	// Clone into store (starts at HEAD = v2.0.0)
	storePath := filepath.Join(dir, ".summon", "local", "store", "test-pkg")
	cloneCmd := exec.Command("git", "clone", "--quiet", remote, storePath)
	require.NoError(t, cloneCmd.Run())

	// Check out v1.0.0 to simulate being at an old version
	checkoutCmd := exec.Command("git", "checkout", "--quiet", "v1.0.0")
	checkoutCmd.Dir = storePath
	require.NoError(t, checkoutCmd.Run())

	oldSHA := gitSHA(t, storePath)

	// Write registry with empty ref (unpinned)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  test-pkg:
    version: "1.0.0"
    source:
      type: github
      url: `+remote+`
      ref: ""
      sha: `+oldSHA+`
    platforms: [claude]
`)

	updateGlobal = false
	updateScope = ""

	captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"test-pkg"})
		assert.NoError(t, err)
	})

	// Verify ref is resolved to latest (v2.0.0)
	reg, err := registry.Load(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	entry, ok := reg.Get("test-pkg")
	require.True(t, ok)
	assert.Equal(t, "v2.0.0", entry.Source.Ref, "unpinned package should resolve to latest tag")
	assert.NotEqual(t, oldSHA, entry.Source.SHA, "SHA should change after updating to latest")
}

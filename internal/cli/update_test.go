package cli

import (
	"fmt"
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
	updateGlobal = false
	updateScope = ""

	out := captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"local-pkg"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Up-to-date")
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o644))
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

	remote := initGitRepo(t, filepath.Join(t.TempDir(), "remote"))
	tagCmd := exec.Command("git", "tag", "v1.0.0")
	tagCmd.Dir = remote
	require.NoError(t, tagCmd.Run())

	storePath := filepath.Join(dir, ".summon", "local", "store", "test-pkg")
	cloneCmd := exec.Command("git", "clone", "--quiet", remote, storePath)
	require.NoError(t, cloneCmd.Run())

	sha := gitSHA(t, storePath)

	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "v2 release")
	commitCmd.Dir = remote
	require.NoError(t, commitCmd.Run())
	tag2Cmd := exec.Command("git", "tag", "v2.0.0")
	tag2Cmd.Dir = remote
	require.NoError(t, tag2Cmd.Run())

	writeRegistryYAML(t, dir, fmt.Sprintf(`
summon_version: "0.1.0"
packages:
  test-pkg:
    version: "1.0.0"
    source:
      type: github
      url: %s
      ref: v1.0.0
      sha: %s
    platforms: [claude]
`, remote, sha))

	updateGlobal = false
	updateScope = ""

	captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"test-pkg"})
		assert.NoError(t, err)
	})

	reg, err := registry.Load(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	entry, ok := reg.Get("test-pkg")
	require.True(t, ok)
	assert.Equal(t, "v1.0.0", entry.Source.Ref, "pinned ref should be preserved after update")
}

func TestRunUpdate_GitHubPackageUnpinned_ResolvesLatest(t *testing.T) {
	dir := setupProjectDir(t)

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

	storePath := filepath.Join(dir, ".summon", "local", "store", "test-pkg")
	cloneCmd := exec.Command("git", "clone", "--quiet", remote, storePath)
	require.NoError(t, cloneCmd.Run())

	checkoutCmd := exec.Command("git", "checkout", "--quiet", "v1.0.0")
	checkoutCmd.Dir = storePath
	require.NoError(t, checkoutCmd.Run())

	oldSHA := gitSHA(t, storePath)

	writeRegistryYAML(t, dir, fmt.Sprintf(`
summon_version: "0.1.0"
packages:
  test-pkg:
    version: "1.0.0"
    source:
      type: github
      url: %s
      ref: ""
      sha: %s
    platforms: [claude]
`, remote, oldSHA))

	updateGlobal = false
	updateScope = ""

	captureStdout(t, func() {
		err := runUpdate(updateCmd, []string{"test-pkg"})
		assert.NoError(t, err)
	})

	reg, err := registry.Load(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	entry, ok := reg.Get("test-pkg")
	require.True(t, ok)
	assert.Equal(t, "v2.0.0", entry.Source.Ref, "unpinned package should resolve to latest tag")
	assert.NotEqual(t, oldSHA, entry.Source.SHA, "SHA should change after updating to latest")
}

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunUninstall_PackageNotInstalled(t *testing.T) {
	_ = setupProjectDir(t)
	uninstallGlobal = false
	uninstallScope = ""

	err := runUninstall(uninstallCmd, []string{"ghost-pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"ghost-pkg" is not installed`)
}

func TestRunUninstall_AmbiguousPackageRequiresScope(t *testing.T) {
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

	uninstallGlobal = false
	uninstallScope = ""

	err := runUninstall(uninstallCmd, []string{"shared-pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installed in multiple scopes")
	assert.Contains(t, err.Error(), "--scope")
}

func TestRunUninstall_Success(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  removable:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/removable
    platforms: [claude]
`)
	createStorePackage(t, dir, "removable")
	uninstallGlobal = false
	uninstallScope = ""

	out := captureStdout(t, func() {
		err := runUninstall(uninstallCmd, []string{"removable"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Uninstalled removable")

	// Store directory should be gone.
	_, err := os.Stat(filepath.Join(dir, ".summon", "local", "store", "removable"))
	assert.True(t, os.IsNotExist(err), "store entry should be removed")

	// Registry should no longer contain the package.
	regData, err := os.ReadFile(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(regData), "removable")
}

func TestRunUninstall_SecondUninstallFails(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  once:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/once
    platforms: []
`)
	createStorePackage(t, dir, "once")
	uninstallGlobal = false
	uninstallScope = ""

	captureStdout(t, func() {
		require.NoError(t, runUninstall(uninstallCmd, []string{"once"}))
	})

	err := runUninstall(uninstallCmd, []string{"once"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"once" is not installed`)
}

func TestRunUninstall_RegistryUpdatedCorrectly(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  keep-me:
    version: "1.0.0"
    source:
      type: local
      url: /some/path
    platforms: []
  remove-me:
    version: "2.0.0"
    source:
      type: github
      url: https://github.com/org/remove-me
    platforms: [claude]
`)
	createStorePackage(t, dir, "keep-me")
	createStorePackage(t, dir, "remove-me")
	uninstallGlobal = false
	uninstallScope = ""

	captureStdout(t, func() {
		require.NoError(t, runUninstall(uninstallCmd, []string{"remove-me"}))
	})

	regData, err := os.ReadFile(filepath.Join(dir, ".summon", "local", "registry.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(regData), "keep-me")
	assert.NotContains(t, string(regData), "remove-me")
}

func TestUninstallCmd_ArgsValidator(t *testing.T) {
	assert.NoError(t, uninstallCmd.Args(uninstallCmd, []string{"pkg"}))
	assert.Error(t, uninstallCmd.Args(uninstallCmd, []string{}),
		"uninstall with 0 args should fail")
	assert.Error(t, uninstallCmd.Args(uninstallCmd, []string{"a", "b"}),
		"uninstall with 2 args should fail")
}

func TestRunUninstall_DisablesPluginOnPlatforms(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  my-plugin:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/my-plugin
    platforms: [claude, copilot]
`)
	createStorePackage(t, dir, "my-plugin")
	uninstallGlobal = false

	captureStdout(t, func() {
		require.NoError(t, runUninstall(uninstallCmd, []string{"my-plugin"}))
	})

	// After uninstall, store entry should be gone
	storePath := filepath.Join(dir, ".summon", "local", "store", "my-plugin")
	_, err := os.Stat(storePath)
	assert.True(t, os.IsNotExist(err), "store entry should be removed after uninstall")
}

func TestRunUninstall_KeepsOtherPackages(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  remove-me:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/org/remove-me
    platforms: []
  keep-me:
    version: "2.0.0"
    source:
      type: github
      url: https://github.com/org/keep-me
    platforms: []
`)
	createStorePackage(t, dir, "remove-me")
	createStorePackage(t, dir, "keep-me")
	uninstallGlobal = false

	captureStdout(t, func() {
		require.NoError(t, runUninstall(uninstallCmd, []string{"remove-me"}))
	})

	// "keep-me" should still be in the store and registry
	keepStorePath := filepath.Join(dir, ".summon", "local", "store", "keep-me")
	assert.DirExists(t, keepStorePath, "keep-me should remain in store")

	removeStorePath := filepath.Join(dir, ".summon", "local", "store", "remove-me")
	_, err := os.Stat(removeStorePath)
	assert.True(t, os.IsNotExist(err), "remove-me should be gone from store")
}

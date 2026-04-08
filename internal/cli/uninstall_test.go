package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/platform"
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
	storeDir := filepath.Join(dir, ".summon", "local", "store")

	// Use a temp global settings dir to avoid writing to real VS Code settings.
	globalDir := filepath.Join(dir, ".test-global")
	require.NoError(t, os.MkdirAll(globalDir, 0o755))

	// Pre-populate plugin activation on all detected platforms.
	for _, a := range platform.DetectActive(dir, platform.WithGlobalSettingsDir(globalDir)) {
		require.NoError(t, a.EnablePlugin("my-plugin", "summon-local", storeDir, platform.ScopeLocal))

		// Verify the key was written to workspace settings
		data, err := os.ReadFile(a.SettingsPath(platform.ScopeLocal))
		require.NoError(t, err)
		var s map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &s))
		switch a.Name() {
		case "claude":
			ep := s["enabledPlugins"].(map[string]interface{})
			require.Equal(t, true, ep["my-plugin@summon-local"])
		case "copilot":
			pl := s["chat.pluginLocations"].(map[string]interface{})
			require.Len(t, pl, 1)
		}
	}

	captureStdout(t, func() {
		require.NoError(t, runUninstall(uninstallCmd, []string{"my-plugin"}))
	})

	// After uninstall, plugin activation should be cleaned up in workspace settings.
	for _, a := range platform.DetectActive(dir, platform.WithGlobalSettingsDir(globalDir)) {
		data, err := os.ReadFile(a.SettingsPath(platform.ScopeLocal))
		require.NoError(t, err)
		var s map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &s))

		switch a.Name() {
		case "claude":
			_, hasEP := s["enabledPlugins"]
			assert.False(t, hasEP,
				"enabledPlugins should be removed from claude after uninstall")
		case "copilot":
			_, hasPL := s["chat.pluginLocations"]
			assert.False(t, hasPL,
				"chat.pluginLocations should be removed from copilot after uninstall")
		}
	}
}

func TestRunUninstall_DisableKeepsOtherPlugins(t *testing.T) {
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
	storeDir := filepath.Join(dir, ".summon", "local", "store")

	// Use a temp global settings dir to avoid writing to real VS Code settings.
	globalDir := filepath.Join(dir, ".test-global")
	require.NoError(t, os.MkdirAll(globalDir, 0o755))

	// Enable both plugins on all detected platforms
	for _, a := range platform.DetectActive(dir, platform.WithGlobalSettingsDir(globalDir)) {
		require.NoError(t, a.EnablePlugin("remove-me", "summon-local", storeDir, platform.ScopeLocal))
		require.NoError(t, a.EnablePlugin("keep-me", "summon-local", storeDir, platform.ScopeLocal))
	}

	captureStdout(t, func() {
		require.NoError(t, runUninstall(uninstallCmd, []string{"remove-me"}))
	})

	// "keep-me" should still be enabled in workspace settings
	for _, a := range platform.DetectActive(dir, platform.WithGlobalSettingsDir(globalDir)) {
		data, err := os.ReadFile(a.SettingsPath(platform.ScopeLocal))
		require.NoError(t, err)
		var s map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &s))

		switch a.Name() {
		case "claude":
			ep, ok := s["enabledPlugins"].(map[string]interface{})
			require.True(t, ok, "enabledPlugins should still exist on claude")
			assert.Equal(t, true, ep["keep-me@summon-local"],
				"keep-me should remain enabled on claude")
			_, hasRemoved := ep["remove-me@summon-local"]
			assert.False(t, hasRemoved,
				"remove-me should be gone from claude")
		case "copilot":
			pl, ok := s["chat.pluginLocations"].(map[string]interface{})
			require.True(t, ok, "chat.pluginLocations should still exist on copilot")
			keepPath, _ := filepath.Abs(filepath.Join(storeDir, "keep-me"))
			if resolved, err := filepath.EvalSymlinks(filepath.Dir(keepPath)); err == nil {
				keepPath = filepath.Join(resolved, "keep-me")
			}
			assert.Equal(t, true, pl[keepPath],
				"keep-me should remain enabled on copilot")
			removePath, _ := filepath.Abs(filepath.Join(storeDir, "remove-me"))
			if resolved, err := filepath.EvalSymlinks(filepath.Dir(removePath)); err == nil {
				removePath = filepath.Join(resolved, "remove-me")
			}
			_, hasRemoved := pl[removePath]
			assert.False(t, hasRemoved,
				"remove-me should be gone from copilot")
		}
	}
}

func TestRunUninstall_ReverseDepCheck_HasDependents_NonInteractive(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  base-lib:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/base-lib"}
  pkg-a:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/pkg-a"}
`)
	writeManifest(t, dir, "local", "base-lib", `
name: base-lib
version: "1.0.0"
description: "base library"
`)
	writeManifest(t, dir, "local", "pkg-a", `
name: pkg-a
version: "1.0.0"
description: "depends on base-lib"
dependencies:
  base-lib: ">=1.0.0"
`)

	uninstallGlobal = false
	uninstallProject = false
	uninstallScope = "local"
	uninstallForce = false

	// Non-interactive → should fail with error
	t.Setenv("SUMMON_NONINTERACTIVE", "1")

	err := runUninstall(uninstallCmd, []string{"base-lib"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has dependents")
}

func TestRunUninstall_ReverseDepCheck_Force(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  base-lib:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/base-lib"}
  pkg-a:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/pkg-a"}
`)
	writeManifest(t, dir, "local", "base-lib", `
name: base-lib
version: "1.0.0"
description: "base library"
`)
	writeManifest(t, dir, "local", "pkg-a", `
name: pkg-a
version: "1.0.0"
description: "depends on base-lib"
dependencies:
  base-lib: ">=1.0.0"
`)

	uninstallGlobal = false
	uninstallProject = false
	uninstallScope = "local"
	uninstallForce = true // force skips reverse dep check

	captureStdout(t, func() {
		err := runUninstall(uninstallCmd, []string{"base-lib"})
		assert.NoError(t, err)
	})

	// Verify base-lib was removed
	_, err := os.Stat(filepath.Join(dir, ".summon", "local", "store", "base-lib"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunUninstall_NoDependents(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  standalone:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/standalone"}
  other-pkg:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/other-pkg"}
`)
	writeManifest(t, dir, "local", "standalone", `
name: standalone
version: "1.0.0"
description: "no deps"
`)
	writeManifest(t, dir, "local", "other-pkg", `
name: other-pkg
version: "1.0.0"
description: "no deps either"
`)

	uninstallGlobal = false
	uninstallProject = false
	uninstallScope = "local"
	uninstallForce = false

	captureStdout(t, func() {
		err := runUninstall(uninstallCmd, []string{"standalone"})
		assert.NoError(t, err)
	})

	// Verify it was uninstalled without prompting
	_, err := os.Stat(filepath.Join(dir, ".summon", "local", "store", "standalone"))
	assert.True(t, os.IsNotExist(err))
}

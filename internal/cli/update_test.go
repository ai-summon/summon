package cli

import (
	"os"
	"path/filepath"
	"testing"

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

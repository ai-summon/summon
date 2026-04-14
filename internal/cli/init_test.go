package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInitCmd returns a fresh init cobra.Command wired to runInit so tests
// don't share package-level flag state.
func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "init",
		RunE: runInit,
	}
	cmd.Flags().StringVar(&initName, "name", "", "")
	cmd.Flags().StringArrayVar(&initPlatform, "platform", nil, "")
	return cmd
}

func TestRunInit_ScaffoldsPackage(t *testing.T) {
	dir := setupProjectDir(t)

	initName = ""
	initPlatform = nil

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--name", "my-pkg"})
	require.NoError(t, cmd.Execute())

	// .claude-plugin/plugin.json should exist with correct name.
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "my-pkg", m["name"])
	assert.Equal(t, "0.1.0", m["version"])

	// Standard subdirectories should exist.
	for _, sub := range []string{"skills", "agents", "commands"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		require.NoError(t, err, "%s/ should be created", sub)
		assert.True(t, info.IsDir())
	}

	// README.md should exist.
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(readme), "# my-pkg")
}

func TestRunInit_DeriveNameFromDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "My_Cool Package")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(sub))
	t.Cleanup(func() { os.Chdir(origDir) })

	initName = ""
	initPlatform = nil

	cmd := newInitCmd()
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(sub, ".claude-plugin", "plugin.json"))
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "my-cool-package", m["name"])
}

func TestRunInit_FailsWhenPluginExists(t *testing.T) {
	dir := setupProjectDir(t)

	pluginDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("{}"), 0o644))

	initName = ""
	initPlatform = nil

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--name", "dup"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRunInit_FailsWhenLegacyManifestExists(t *testing.T) {
	dir := setupProjectDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte("existing"), 0o644))

	initName = ""
	initPlatform = nil

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--name", "legacy"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "summon.yaml already exists")
}

func TestRunInit_PreservesExistingReadme(t *testing.T) {
	dir := setupProjectDir(t)

	existing := "# Custom Readme\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(existing), 0o644))

	initName = ""
	initPlatform = nil

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--name", "keep-readme"})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(data), "existing README.md should not be overwritten")
}

func TestRunInit_PreservesExistingSubdirectories(t *testing.T) {
	dir := setupProjectDir(t)

	skillsDir := filepath.Join(dir, "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "existing.md"), []byte("keep"), 0o644))

	initName = ""
	initPlatform = nil

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--name", "keep-dirs"})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(skillsDir, "existing.md"))
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data), "files in pre-existing subdirectories should be preserved")
}

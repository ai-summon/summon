package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestData := `name: my-plugin
description: A test plugin
dependencies:
  - other-plugin
  - gh:owner/dep
system_requirements:
  - git
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	deps := &validateDeps{
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	validateJSON = false
	err := runValidate(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "✓ summon.yaml syntax is valid")
}

func TestValidate_MissingFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	deps := &validateDeps{
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no summon.yaml")
}

func TestValidate_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestData := `name: my-plugin
# missing description
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	deps := &validateDeps{
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)
}

func TestValidate_MissingRequiredSysReq(t *testing.T) {
	dir := t.TempDir()
	manifestData := `name: my-plugin
description: A test plugin
system_requirements:
  - nonexistent-binary-xyz-test
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	deps := &validateDeps{
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "✗")
	assert.Contains(t, out, "nonexistent-binary-xyz-test")
}

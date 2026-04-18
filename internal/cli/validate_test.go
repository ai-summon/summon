package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestData := `dependencies:
  - other-plugin
  - gh:owner/dep
system_requirements:
  - git
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	deps := &validateDeps{
noColor: true,
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
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no summon.yaml")
}

func TestValidate_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestData := `system_requirements:
  - name: docker
    optional: true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)
}

func TestValidate_MissingRequiredSysReq(t *testing.T) {
	dir := t.TempDir()
	manifestData := `system_requirements:
  - nonexistent-binary-xyz-test
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "✗")
	assert.Contains(t, out, "nonexistent-binary-xyz-test")
}

func TestValidate_JSON_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	manifestData := `dependencies:
  - other-plugin
  - gh:owner/dep
system_requirements:
  - git
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	validateJSON = true
	defer func() { validateJSON = false }()

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).Bytes()
	var result ValidateOutput
	require.NoError(t, json.Unmarshal(out, &result), "output should be valid JSON")

	assert.NotEmpty(t, result.Results)
	assert.Equal(t, "syntax", result.Results[0].Check)
	assert.Equal(t, "pass", result.Results[0].Status)
	assert.Greater(t, result.Summary.Total, 0)
	assert.Equal(t, 0, result.Summary.Failed)
	assert.Equal(t, result.Summary.Total, result.Summary.Passed+result.Summary.Warnings)
}

func TestValidate_JSON_WithErrors(t *testing.T) {
	dir := t.TempDir()
	manifestData := `system_requirements:
  - nonexistent-binary-xyz-test
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	validateJSON = true
	defer func() { validateJSON = false }()

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	assert.Error(t, err)

	out := deps.stdout.(*bytes.Buffer).Bytes()
	var result ValidateOutput
	require.NoError(t, json.Unmarshal(out, &result), "output should be valid JSON even with errors")

	assert.Greater(t, result.Summary.Failed, 0)
	// Verify failure details are present
	hasFail := false
	for _, r := range result.Results {
		if r.Status == "fail" {
			hasFail = true
			assert.Contains(t, r.Message, "nonexistent-binary-xyz-test")
		}
	}
	assert.True(t, hasFail, "should have at least one failed result")
}

func TestValidate_JSON_NoHumanOutput(t *testing.T) {
	dir := t.TempDir()
	manifestData := `dependencies:
  - other-plugin
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	validateJSON = true
	defer func() { validateJSON = false }()

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	// JSON mode should not contain human-readable symbols
	assert.NotContains(t, out, "✓")
	assert.NotContains(t, out, "✗")
	assert.NotContains(t, out, "⚠")
	// Should be pure JSON
	assert.True(t, json.Valid([]byte(out)), "output should be valid JSON")
}

func TestValidate_WithoutJSON_StillHumanReadable(t *testing.T) {
	dir := t.TempDir()
	manifestData := `dependencies:
  - other-plugin
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	validateJSON = false

	deps := &validateDeps{
noColor: true,
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	err := runValidate(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "✓")
	assert.Contains(t, out, "0 error(s)")
	assert.False(t, json.Valid([]byte(out)), "human output should not be valid JSON")
}

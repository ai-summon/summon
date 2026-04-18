package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_ValidManifest(t *testing.T) {
	data := []byte(`
marketplaces:
  some-marketplace: gh:owner/marketplace-repo
dependencies:
  - other-plugin
  - cool-tool@some-marketplace
  - gh:owner/repo
system_requirements:
  - python3
  - name: docker
    optional: true
    reason: "Only needed for containerized execution"
`)
	m, err := ParseAndValidate(data)
	require.NoError(t, err)
	assert.Equal(t, "gh:owner/marketplace-repo", m.Marketplaces["some-marketplace"])
	assert.Len(t, m.Dependencies, 3)
	assert.Equal(t, "other-plugin", m.Dependencies[0])
	assert.Len(t, m.SystemRequirements, 2)

	assert.Equal(t, "python3", m.SystemRequirements[0].Name)
	assert.False(t, m.SystemRequirements[0].Optional)

	assert.Equal(t, "docker", m.SystemRequirements[1].Name)
	assert.True(t, m.SystemRequirements[1].Optional)
	assert.Equal(t, "Only needed for containerized execution", m.SystemRequirements[1].Reason)
}

func TestParse_OptionalWithoutReason(t *testing.T) {
	data := []byte(`
system_requirements:
  - name: docker
    optional: true
`)
	_, err := ParseAndValidate(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "optional but missing 'reason'")
}

func TestParse_StringSystemRequirement(t *testing.T) {
	data := []byte(`
system_requirements:
  - python3
  - git
`)
	m, err := ParseAndValidate(data)
	require.NoError(t, err)
	assert.Len(t, m.SystemRequirements, 2)
	assert.Equal(t, "python3", m.SystemRequirements[0].Name)
	assert.False(t, m.SystemRequirements[0].Optional)
	assert.Equal(t, "git", m.SystemRequirements[1].Name)
	assert.False(t, m.SystemRequirements[1].Optional)
}

func TestParse_MinimalValid(t *testing.T) {
	data := []byte(`
dependencies:
  - some-plugin
`)
	m, err := ParseAndValidate(data)
	require.NoError(t, err)
	assert.Len(t, m.Dependencies, 1)
	assert.Empty(t, m.SystemRequirements)
}

func TestParse_EmptyManifest(t *testing.T) {
	data := []byte(`{}`)
	m, err := ParseAndValidate(data)
	require.NoError(t, err)
	assert.Empty(t, m.Dependencies)
	assert.Empty(t, m.SystemRequirements)
}

func TestParse_InvalidYAML(t *testing.T) {
	data := []byte(`{{{invalid yaml`)
	_, err := Parse(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest parse error")
}

func TestValidate_EmptySystemRequirementName(t *testing.T) {
	m := &Manifest{
		SystemRequirements: []SystemRequirement{
			{Name: ""},
		},
	}
	err := m.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have a 'name'")
}

func TestParseAndValidate_InvalidYAML(t *testing.T) {
	data := []byte(`{{{invalid yaml`)
	_, err := ParseAndValidate(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest parse error")
}

func TestLoadFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summon.yaml")
	require.NoError(t, os.WriteFile(path, []byte("dependencies:\n  - some-plugin\n"), 0644))

	m, err := LoadFile(path)
	require.NoError(t, err)
	assert.Contains(t, m.Dependencies, "some-plugin")
}

func TestLoadFile_NonexistentFile(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/summon.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest load error")
}

func TestLoadFile_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summon.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`{{{invalid yaml`), 0644))

	_, err := LoadFile(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest parse error")
}

func TestLoadFile_ValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summon.yaml")
	require.NoError(t, os.WriteFile(path, []byte("system_requirements:\n  - name: docker\n    optional: true\n"), 0644))

	_, err := LoadFile(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "optional but missing 'reason'")
}

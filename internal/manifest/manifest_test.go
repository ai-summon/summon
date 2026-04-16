package manifest

import (
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

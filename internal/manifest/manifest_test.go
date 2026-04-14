package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_ValidManifest(t *testing.T) {
	data := []byte(`
name: my-plugin
description: A useful plugin
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
	assert.Equal(t, "my-plugin", m.Name)
	assert.Equal(t, "A useful plugin", m.Description)
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

func TestParse_MissingName(t *testing.T) {
	data := []byte(`
description: A plugin
`)
	_, err := ParseAndValidate(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "'name' is required")
}

func TestParse_MissingDescription(t *testing.T) {
	data := []byte(`
name: my-plugin
`)
	_, err := ParseAndValidate(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "'description' is required")
}

func TestParse_OptionalWithoutReason(t *testing.T) {
	data := []byte(`
name: my-plugin
description: A plugin
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
name: my-plugin
description: A plugin
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

func TestParse_InvalidNameFormat(t *testing.T) {
	data := []byte(`
name: My_Plugin
description: A plugin
`)
	_, err := ParseAndValidate(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kebab-case")
}

func TestParse_NameTooLong(t *testing.T) {
	longName := "a"
	for i := 0; i < 50; i++ {
		longName += "b"
	}
	data := []byte("name: " + longName + "\ndescription: A plugin\n")
	_, err := ParseAndValidate(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "50 characters")
}

func TestParse_MinimalValid(t *testing.T) {
	data := []byte(`
name: simple
description: Minimal valid manifest
`)
	m, err := ParseAndValidate(data)
	require.NoError(t, err)
	assert.Equal(t, "simple", m.Name)
	assert.Empty(t, m.Dependencies)
	assert.Empty(t, m.SystemRequirements)
}

package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePluginJSON_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	m := &manifest.Manifest{
		Name:        "test-plugin",
		Description: "A test plugin",
		Version:     "1.2.3",
		License:     "MIT",
	}

	err := GeneratePluginJSON(dir, m)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	require.NoError(t, err)

	var p PluginJSON
	require.NoError(t, json.Unmarshal(data, &p))

	assert.Equal(t, "test-plugin", p.Name)
	assert.Equal(t, "A test plugin", p.Description)
	assert.Equal(t, "1.2.3", p.Version)
	assert.Equal(t, "MIT", p.License)
	assert.Nil(t, p.Author)
}

func TestGeneratePluginJSON_WithAuthor(t *testing.T) {
	dir := t.TempDir()
	m := &manifest.Manifest{
		Name:        "authored-plugin",
		Description: "Has author",
		Version:     "0.1.0",
		Author:      &manifest.Author{Name: "Jane Doe", Email: "jane@example.com"},
	}

	err := GeneratePluginJSON(dir, m)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	require.NoError(t, err)

	var p PluginJSON
	require.NoError(t, json.Unmarshal(data, &p))

	require.NotNil(t, p.Author)
	assert.Equal(t, "Jane Doe", p.Author.Name)
	assert.Equal(t, "jane@example.com", p.Author.Email)
}

func TestGeneratePluginJSON_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	m1 := &manifest.Manifest{
		Name:        "plugin-v1",
		Description: "Version 1",
		Version:     "1.0.0",
	}
	m2 := &manifest.Manifest{
		Name:        "plugin-v2",
		Description: "Version 2",
		Version:     "2.0.0",
	}

	require.NoError(t, GeneratePluginJSON(dir, m1))
	require.NoError(t, GeneratePluginJSON(dir, m2))

	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	require.NoError(t, err)

	var p PluginJSON
	require.NoError(t, json.Unmarshal(data, &p))

	assert.Equal(t, "plugin-v2", p.Name)
	assert.Equal(t, "2.0.0", p.Version)
}

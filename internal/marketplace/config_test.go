package marketplace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_NotExists(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/config.yaml")
	require.NoError(t, err)
	assert.Empty(t, cfg.Marketplaces)
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Marketplaces: []MarketplaceEntry{
			{Name: "my-marketplace", Source: "https://github.com/org/marketplace"},
		},
	}

	require.NoError(t, SaveConfig(path, cfg))

	loaded, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Marketplaces, 1)
	assert.Equal(t, "my-marketplace", loaded.Marketplaces[0].Name)
}

func TestAddMarketplace(t *testing.T) {
	cfg := &Config{}
	err := cfg.AddMarketplace("test", "https://example.com/marketplace")
	require.NoError(t, err)
	assert.Len(t, cfg.Marketplaces, 1)
}

func TestAddMarketplace_Duplicate(t *testing.T) {
	cfg := &Config{
		Marketplaces: []MarketplaceEntry{
			{Name: "test", Source: "https://example.com"},
		},
	}
	err := cfg.AddMarketplace("test", "https://other.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRemoveMarketplace(t *testing.T) {
	cfg := &Config{
		Marketplaces: []MarketplaceEntry{
			{Name: "test", Source: "https://example.com"},
		},
	}
	err := cfg.RemoveMarketplace("test")
	require.NoError(t, err)
	assert.Empty(t, cfg.Marketplaces)
}

func TestRemoveMarketplace_NotFound(t *testing.T) {
	cfg := &Config{}
	err := cfg.RemoveMarketplace("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindMarketplace(t *testing.T) {
	cfg := &Config{
		Marketplaces: []MarketplaceEntry{
			{Name: "test", Source: "https://example.com"},
		},
	}
	m := cfg.FindMarketplace("test")
	assert.NotNil(t, m)
	assert.Equal(t, "https://example.com", m.Source)
}

func TestFindMarketplace_NotFound(t *testing.T) {
	cfg := &Config{}
	m := cfg.FindMarketplace("nonexistent")
	assert.Nil(t, m)
}

func TestSaveConfig_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	cfg := &Config{}
	require.NoError(t, SaveConfig(path, cfg))

	_, err := os.Stat(filepath.Dir(path))
	assert.NoError(t, err)
}

func TestLoadConfig_CorruptYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{{{invalid"), 0644))

	_, err := LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalManifestFetcher(t *testing.T) {
	f := NewLocalManifestFetcher()
	assert.NotNil(t, f)
}

func TestLocalFetchManifest_WithValidManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte("dependencies:\n  - some-dep\n"), 0644))

	f := NewLocalManifestFetcher()
	m, err := f.FetchManifest(dir)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Contains(t, m.Dependencies, "some-dep")
}

func TestLocalFetchManifest_NoManifest(t *testing.T) {
	dir := t.TempDir()

	f := NewLocalManifestFetcher()
	m, err := f.FetchManifest(dir)
	require.NoError(t, err)
	assert.Nil(t, m)
}

func TestLocalFetchManifest_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(`{{{invalid yaml`), 0644))

	f := NewLocalManifestFetcher()
	_, err := f.FetchManifest(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest parse error")
}

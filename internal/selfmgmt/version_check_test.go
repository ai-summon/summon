package selfmgmt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckVersionCache_NoCacheFile(t *testing.T) {
	dir := t.TempDir()
	result := CheckVersionCache(dir, "0.1.0")
	assert.False(t, result.UpdateAvailable)
	assert.True(t, result.NeedsRefresh)
	assert.Equal(t, "0.1.0", result.CurrentVersion)
}

func TestCheckVersionCache_FreshCacheWithNewerVersion(t *testing.T) {
	dir := t.TempDir()
	writeTestCache(t, dir, VersionCache{
		LatestVersion: "0.2.0",
		CheckedAt:     time.Now().UTC(),
	})

	result := CheckVersionCache(dir, "v0.1.0")
	assert.True(t, result.UpdateAvailable)
	assert.False(t, result.NeedsRefresh)
	assert.Equal(t, "0.1.0", result.CurrentVersion)
	assert.Equal(t, "0.2.0", result.LatestVersion)
}

func TestCheckVersionCache_FreshCacheAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	writeTestCache(t, dir, VersionCache{
		LatestVersion: "0.1.0",
		CheckedAt:     time.Now().UTC(),
	})

	result := CheckVersionCache(dir, "0.1.0")
	assert.False(t, result.UpdateAvailable)
	assert.False(t, result.NeedsRefresh)
}

func TestCheckVersionCache_StaleCache(t *testing.T) {
	dir := t.TempDir()
	writeTestCache(t, dir, VersionCache{
		LatestVersion: "0.2.0",
		CheckedAt:     time.Now().UTC().Add(-25 * time.Hour),
	})

	result := CheckVersionCache(dir, "0.1.0")
	assert.True(t, result.UpdateAvailable, "should still report update even with stale cache")
	assert.True(t, result.NeedsRefresh, "should request refresh for stale cache")
	assert.Equal(t, "0.2.0", result.LatestVersion)
}

func TestCheckVersionCache_DevVersion(t *testing.T) {
	dir := t.TempDir()
	result := CheckVersionCache(dir, "dev")
	assert.False(t, result.UpdateAvailable)
	assert.False(t, result.NeedsRefresh, "should not refresh for dev builds")
}

func TestCheckVersionCache_EmptyVersion(t *testing.T) {
	dir := t.TempDir()
	result := CheckVersionCache(dir, "")
	assert.False(t, result.UpdateAvailable)
	assert.False(t, result.NeedsRefresh)
}

func TestCheckVersionCache_InvalidCachedVersion(t *testing.T) {
	dir := t.TempDir()
	writeTestCache(t, dir, VersionCache{
		LatestVersion: "not-a-version",
		CheckedAt:     time.Now().UTC(),
	})

	result := CheckVersionCache(dir, "0.1.0")
	assert.False(t, result.UpdateAvailable)
	assert.True(t, result.NeedsRefresh, "should request refresh for invalid cached version")
}

func TestCheckVersionCache_CorruptedCacheFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, versionCacheFile), []byte("{bad json"), 0600))

	result := CheckVersionCache(dir, "0.1.0")
	assert.False(t, result.UpdateAvailable)
	assert.True(t, result.NeedsRefresh, "should request refresh for corrupted cache")
}

func TestCheckVersionCache_SemverComparison(t *testing.T) {
	tests := []struct {
		name            string
		current         string
		latest          string
		updateAvailable bool
	}{
		{"major upgrade", "0.1.0", "1.0.0", true},
		{"minor upgrade", "0.1.0", "0.2.0", true},
		{"patch upgrade", "0.1.0", "0.1.1", true},
		{"same version", "0.1.0", "0.1.0", false},
		{"current is newer", "0.2.0", "0.1.0", false},
		{"double-digit minor", "0.2.0", "0.10.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestCache(t, dir, VersionCache{
				LatestVersion: tt.latest,
				CheckedAt:     time.Now().UTC(),
			})

			result := CheckVersionCache(dir, tt.current)
			assert.Equal(t, tt.updateAvailable, result.UpdateAvailable)
		})
	}
}

func TestRefreshVersionCache_Success(t *testing.T) {
	clearURLEnvVars(t)

	dir := t.TempDir()
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.3.0"}`)

	RefreshVersionCache(dir, client)

	cache, err := readCache(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.3.0", cache.LatestVersion)
	assert.WithinDuration(t, time.Now().UTC(), cache.CheckedAt, 5*time.Second)
}

func TestRefreshVersionCache_NetworkError(t *testing.T) {
	clearURLEnvVars(t)

	dir := t.TempDir()
	client := newFakeHTTPClient()
	client.errors[defaultReleasesAPI] = fmt.Errorf("network error")

	RefreshVersionCache(dir, client)

	// Cache file should not exist after a failed refresh.
	_, err := readCache(dir)
	assert.Error(t, err)
}

func TestRefreshVersionCache_CreatesConfigDir(t *testing.T) {
	clearURLEnvVars(t)

	dir := filepath.Join(t.TempDir(), "nonexistent", ".summon")
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.5.0"}`)

	RefreshVersionCache(dir, client)

	cache, err := readCache(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.5.0", cache.LatestVersion)
}

func TestRefreshVersionCache_OverwritesExistingCache(t *testing.T) {
	clearURLEnvVars(t)

	dir := t.TempDir()
	writeTestCache(t, dir, VersionCache{
		LatestVersion: "0.1.0",
		CheckedAt:     time.Now().UTC().Add(-48 * time.Hour),
	})

	client := &fakeHTTPClient{
		responses: map[string]*http.Response{
			defaultReleasesAPI: {
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v0.4.0"}`)),
			},
		},
		errors: make(map[string]error),
	}

	RefreshVersionCache(dir, client)

	cache, err := readCache(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.4.0", cache.LatestVersion)
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0.1.0", "v0.1.0"},
		{"v0.1.0", "v0.1.0"},
		{"dev", ""},
		{"", ""},
		{"not-semver", ""},
		{"1.2.3", "v1.2.3"},
		{"v1.2.3-rc.1", "v1.2.3-rc.1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeVersion(tt.input))
		})
	}
}

// writeTestCache writes a VersionCache to the cache file for testing.
func writeTestCache(t *testing.T, dir string, cache VersionCache) {
	t.Helper()
	data, err := json.Marshal(cache)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, versionCacheFile), data, 0600))
}

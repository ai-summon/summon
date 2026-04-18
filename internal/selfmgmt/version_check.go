package selfmgmt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"
)

const (
	versionCacheFile = "version-check.json"
	checkInterval    = 24 * time.Hour
)

// VersionCache stores the result of the latest version check.
type VersionCache struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

// VersionCheckResult holds the outcome of a version check against the cache.
type VersionCheckResult struct {
	// UpdateAvailable is true when a newer version exists.
	UpdateAvailable bool
	// CurrentVersion is the running version (without "v" prefix).
	CurrentVersion string
	// LatestVersion is the newest available version (without "v" prefix).
	LatestVersion string
	// NeedsRefresh is true when the cache is stale or missing.
	NeedsRefresh bool
}

// CheckVersionCache reads the cache file and compares the current version
// against the cached latest version. It does no network I/O.
func CheckVersionCache(configDir, currentVersion string) VersionCheckResult {
	current := normalizeVersion(currentVersion)
	result := VersionCheckResult{CurrentVersion: StripVersion(currentVersion)}

	if current == "" {
		return result
	}

	cache, err := readCache(configDir)
	if err != nil || cache.LatestVersion == "" {
		result.NeedsRefresh = true
		return result
	}

	if time.Since(cache.CheckedAt) > checkInterval {
		result.NeedsRefresh = true
	}

	latest := normalizeVersion(cache.LatestVersion)
	if latest == "" {
		result.NeedsRefresh = true
		return result
	}

	if semver.Compare(latest, current) > 0 {
		result.UpdateAvailable = true
		result.LatestVersion = StripVersion(cache.LatestVersion)
	}

	return result
}

// RefreshVersionCache fetches the latest version from GitHub and writes the
// cache file. Errors are silently ignored (best-effort).
func RefreshVersionCache(configDir string, httpClient HTTPClient) {
	release, err := FetchLatestVersion(httpClient)
	if err != nil {
		return
	}

	cache := VersionCache{
		LatestVersion: release.Version,
		CheckedAt:     time.Now().UTC(),
	}

	writeCache(configDir, cache)
}

// normalizeVersion ensures the version has a "v" prefix and is valid semver.
// Returns "" for invalid or dev versions.
func normalizeVersion(v string) string {
	v = StripVersion(v)
	if v == "" || v == "dev" {
		return ""
	}
	canonical := "v" + v
	if !semver.IsValid(canonical) {
		return ""
	}
	return canonical
}

func cacheFilePath(configDir string) string {
	return filepath.Join(configDir, versionCacheFile)
}

func readCache(configDir string) (VersionCache, error) {
	data, err := os.ReadFile(cacheFilePath(configDir))
	if err != nil {
		return VersionCache{}, err
	}
	var cache VersionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return VersionCache{}, err
	}
	return cache, nil
}

func writeCache(configDir string, cache VersionCache) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	// Atomic write: write to temp file then rename.
	tmp := cacheFilePath(configDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, cacheFilePath(configDir))
}

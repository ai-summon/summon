package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-summon/summon/internal/selfmgmt"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunVersionCheck_PrintsWarning(t *testing.T) {
	// Capture stderr.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	writeVersionCache(t, dir, "0.2.0", time.Now().UTC())

	cmd := &cobra.Command{Use: "install"}
	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  dir,
		isTTY:      func() bool { return true },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	output := string(out)
	assert.Contains(t, output, "Update available")
	assert.Contains(t, output, "v0.1.0")
	assert.Contains(t, output, "v0.2.0")
	assert.Contains(t, output, "summon self update")
}

func TestRunVersionCheck_NoWarningWhenCurrent(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.2.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	writeVersionCache(t, dir, "0.2.0", time.Now().UTC())

	cmd := &cobra.Command{Use: "install"}
	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  dir,
		isTTY:      func() bool { return true },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	assert.Empty(t, string(out))
}

func TestRunVersionCheck_SkippedWhenDisabled(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "1")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	writeVersionCache(t, dir, "0.2.0", time.Now().UTC())

	cmd := &cobra.Command{Use: "install"}
	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  dir,
		isTTY:      func() bool { return true },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	assert.Empty(t, string(out))
}

func TestRunVersionCheck_SkippedWhenDevVersion(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "dev"
	defer func() { rootCmd.Version = oldVersion }()

	cmd := &cobra.Command{Use: "install"}
	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  t.TempDir(),
		isTTY:      func() bool { return true },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	assert.Empty(t, string(out))
}

func TestRunVersionCheck_SkippedWhenNotTTY(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	writeVersionCache(t, dir, "0.2.0", time.Now().UTC())

	cmd := &cobra.Command{Use: "install"}
	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  dir,
		isTTY:      func() bool { return false },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	assert.Empty(t, string(out))
}

func TestRunVersionCheck_SkippedForSelfUpdate(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	writeVersionCache(t, dir, "0.2.0", time.Now().UTC())

	parent := &cobra.Command{Use: "self"}
	cmd := &cobra.Command{Use: "update"}
	parent.AddCommand(cmd)

	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  dir,
		isTTY:      func() bool { return true },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	assert.Empty(t, string(out))
}

func TestRunVersionCheck_SkippedForHelp(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	writeVersionCache(t, dir, "0.2.0", time.Now().UTC())

	cmd := &cobra.Command{Use: "help"}
	deps := &versionCheckDeps{
		httpClient: &http.Client{},
		configDir:  dir,
		isTTY:      func() bool { return true },
	}

	runVersionCheck(cmd, deps)

	w.Close()
	out, _ := io.ReadAll(r)
	assert.Empty(t, string(out))
}

func TestRunVersionCheck_RefreshesStaleCache(t *testing.T) {
	t.Setenv("SUMMON_NO_UPDATE_CHECK", "")
	t.Setenv("SUMMON_GITHUB_API", "")
	t.Setenv("SUMMON_DOWNLOAD_BASE", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	dir := t.TempDir()
	// Write a stale cache (25 hours old).
	writeVersionCache(t, dir, "0.1.0", time.Now().UTC().Add(-25*time.Hour))

	client := &fakeVersionCheckHTTPClient{
		response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v0.3.0"}`)),
		},
	}

	cmd := &cobra.Command{Use: "install"}
	deps := &versionCheckDeps{
		httpClient: client,
		configDir:  dir,
		isTTY:      func() bool { return true },
	}

	// Redirect stderr to avoid test output noise.
	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	runVersionCheck(cmd, deps)
	w.Close()

	// Give the goroutine a moment to write the cache.
	time.Sleep(100 * time.Millisecond)

	// Verify cache was updated.
	data, err := os.ReadFile(filepath.Join(dir, "version-check.json"))
	require.NoError(t, err)

	var cache selfmgmt.VersionCache
	require.NoError(t, json.Unmarshal(data, &cache))
	assert.Equal(t, "0.3.0", cache.LatestVersion)
}

func TestShouldSkipVersionCheck(t *testing.T) {
	tests := []struct {
		name   string
		cmd    *cobra.Command
		parent *cobra.Command
		skip   bool
	}{
		{"regular command", &cobra.Command{Use: "install"}, nil, false},
		{"help command", &cobra.Command{Use: "help"}, nil, true},
		{"completion command", &cobra.Command{Use: "completion"}, nil, true},
		{"self update", &cobra.Command{Use: "update"}, &cobra.Command{Use: "self"}, true},
		{"non-self update", &cobra.Command{Use: "update"}, &cobra.Command{Use: "summon"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.parent != nil {
				tt.parent.AddCommand(tt.cmd)
			}
			assert.Equal(t, tt.skip, shouldSkipVersionCheck(tt.cmd))
		})
	}
}

// fakeVersionCheckHTTPClient is a simple test double.
type fakeVersionCheckHTTPClient struct {
	response *http.Response
	err      error
}

func (f *fakeVersionCheckHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func writeVersionCache(t *testing.T, dir string, version string, checkedAt time.Time) {
	t.Helper()
	cache := selfmgmt.VersionCache{
		LatestVersion: version,
		CheckedAt:     checkedAt,
	}
	data, err := json.Marshal(cache)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version-check.json"), data, 0600))
}

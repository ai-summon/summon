package selfmgmt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// HTTPClient abstracts HTTP requests for testability.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ExecRunner abstracts subprocess execution for testability.
type ExecRunner interface {
	RunWithEnv(name string, args []string, env []string, stdout, stderr io.Writer) error
}

// ExecRunnerAdapter is the default ExecRunner using os/exec.
type ExecRunnerAdapter struct{}

func (o *ExecRunnerAdapter) RunWithEnv(name string, args []string, env []string, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// ReleaseInfo holds information about a GitHub release.
type ReleaseInfo struct {
	TagName string // Version tag (e.g., "v0.1.0")
	Version string // Stripped version (e.g., "0.1.0")
}

// UpdateResult represents the outcome of a self-update operation.
type UpdateResult struct {
	CurrentVersion  string
	LatestVersion   string
	AlreadyUpToDate bool
	Updated         bool
}

// githubRelease is the subset of the GitHub release API response we need.
type githubRelease struct {
	TagName string `json:"tag_name"`
}

const (
	defaultReleasesAPI = "https://api.github.com/repos/ai-summon/summon/releases/latest"
	defaultRawGitHub   = "https://raw.githubusercontent.com/ai-summon/summon"
)

// getReleasesAPI returns the GitHub Releases API URL.
// Checks SUMMON_GITHUB_API env var first, falls back to the default.
func getReleasesAPI() string {
	if v := os.Getenv("SUMMON_GITHUB_API"); v != "" {
		return v
	}
	return defaultReleasesAPI
}

// getRawGitHub returns the base URL for downloading raw content (installer scripts).
// Checks SUMMON_DOWNLOAD_BASE env var first, falls back to the default.
func getRawGitHub() string {
	if v := os.Getenv("SUMMON_DOWNLOAD_BASE"); v != "" {
		return v
	}
	return defaultRawGitHub
}

// FetchLatestVersion queries the GitHub Releases API for the latest release.
func FetchLatestVersion(httpClient HTTPClient) (ReleaseInfo, error) {
	req, err := http.NewRequest("GET", getReleasesAPI(), nil)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "summon-self-update")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("failed to check for updates: %w", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ReleaseInfo{}, fmt.Errorf("no releases found for ai-summon/summon")
	}
	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ReleaseInfo{}, fmt.Errorf("failed to parse release information: %w", err)
	}

	if release.TagName == "" {
		return ReleaseInfo{}, fmt.Errorf("no releases found for ai-summon/summon")
	}

	return ReleaseInfo{
		TagName: release.TagName,
		Version: strings.TrimPrefix(release.TagName, "v"),
	}, nil
}

// installerScriptName returns the installer script name for the current platform.
func installerScriptName() string {
	if runtime.GOOS == "windows" {
		return "install.ps1"
	}
	return "install.sh"
}

// installerCommand returns the command and args to execute the installer.
func installerCommand(scriptPath string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-ExecutionPolicy", "Bypass", "-File", scriptPath}
	}
	return "sh", []string{scriptPath}
}

// StripVersion removes the "v" prefix from a version string.
func StripVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

// IsUpToDate returns true if the current and latest versions match.
func IsUpToDate(currentVersion, latestVersion string) bool {
	return StripVersion(currentVersion) == StripVersion(latestVersion)
}

// RunUpdate checks for a newer version and updates the binary if available.
//
// Deprecated: Use FetchLatestVersion + IsUpToDate + PerformUpdate for better control.
func RunUpdate(currentVersion string, paths SummonPaths, httpClient HTTPClient, runner ExecRunner, w io.Writer) (*UpdateResult, error) {
	current := StripVersion(currentVersion)

	release, err := FetchLatestVersion(httpClient)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{
		CurrentVersion: current,
		LatestVersion:  release.Version,
	}

	if IsUpToDate(currentVersion, release.Version) {
		result.AlreadyUpToDate = true
		return result, nil
	}

	_, _ = fmt.Fprintf(w, "updating summon v%s → v%s\n", current, release.Version)

	if err := PerformUpdate(release, paths, httpClient, runner, w); err != nil {
		return nil, err
	}

	result.Updated = true
	return result, nil
}

// PerformUpdate downloads and installs a specific release version.
// It assumes the caller has already determined that an update is needed.
func PerformUpdate(release ReleaseInfo, paths SummonPaths, httpClient HTTPClient, runner ExecRunner, w io.Writer) error {
	// Download installer script first, before any destructive operations
	scriptName := installerScriptName()
	scriptURL := fmt.Sprintf("%s/%s/%s", getRawGitHub(), release.TagName, scriptName)

	req, err := http.NewRequest("GET", scriptURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("User-Agent", "summon-self-update")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("update failed: %w\nthe current installation has not been modified", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update failed: could not download installer (HTTP %d)\nthe current installation has not been modified", resp.StatusCode)
	}

	scriptContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("update failed: could not read installer script\nthe current installation has not been modified")
	}

	tmpFile, err := createTempScript(scriptContent, scriptName)
	if err != nil {
		return fmt.Errorf("update failed: could not write installer script: %w", err)
	}
	defer removeTempFile(tmpFile)

	// Platform-specific preparation (rename on Windows, no-op on Unix).
	// Done after download so we don't modify the binary if download fails.
	if err := PrepareForUpdate(paths.BinaryPath); err != nil {
		return fmt.Errorf("failed to prepare for update: %w", err)
	}

	cmdName, cmdArgs := installerCommand(tmpFile)
	env := buildInstallerEnv(paths.BinaryDir, release.TagName)

	if err := runner.RunWithEnv(cmdName, cmdArgs, env, io.Discard, w); err != nil {
		return fmt.Errorf("update failed: installer exited with error")
	}

	return nil
}

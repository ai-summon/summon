package selfmgmt

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func newFakeHTTPClient() *fakeHTTPClient {
	return &fakeHTTPClient{
		responses: make(map[string]*http.Response),
		errors:    make(map[string]error),
	}
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	if err, ok := f.errors[url]; ok {
		return nil, err
	}
	if resp, ok := f.responses[url]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (f *fakeHTTPClient) setJSON(url string, statusCode int, body string) {
	f.responses[url] = &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// clearURLEnvVars ensures tests use default URLs regardless of host environment.
func clearURLEnvVars(t *testing.T) {
	t.Setenv("SUMMON_GITHUB_API", "")
	t.Setenv("SUMMON_DOWNLOAD_BASE", "")
}

// tempSummonPaths creates a temp directory with a fake binary file and returns
// SummonPaths pointing to it. This is needed because PrepareForUpdate on Windows
// performs a real os.Rename on the binary path.
func tempSummonPaths(t *testing.T) SummonPaths {
	t.Helper()
	dir := t.TempDir()
	name := "summon"
	if runtime.GOOS == "windows" {
		name = "summon.exe"
	}
	binaryPath := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0755))
	return SummonPaths{
		BinaryPath: binaryPath,
		BinaryDir:  dir,
		ConfigDir:  filepath.Join(dir, ".summon"),
	}
}

type fakeExecRunner struct {
	commands []string
	err      error
}

func (f *fakeExecRunner) RunWithEnv(name string, args []string, env []string, stdout, stderr io.Writer) error {
	f.commands = append(f.commands, name+" "+strings.Join(args, " "))
	return f.err
}

func TestFetchLatestVersion_Success(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.2.0"}`)

	info, err := FetchLatestVersion(client)
	require.NoError(t, err)
	assert.Equal(t, "v0.2.0", info.TagName)
	assert.Equal(t, "0.2.0", info.Version)
}

func TestFetchLatestVersion_NetworkError(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.errors[defaultReleasesAPI] = fmt.Errorf("dial tcp: lookup api.github.com: no such host")

	_, err := FetchLatestVersion(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for updates")
}

func TestFetchLatestVersion_NoReleases(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 404, ``)

	_, err := FetchLatestVersion(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no releases found")
}

func TestFetchLatestVersion_MalformedJSON(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{invalid json`)

	_, err := FetchLatestVersion(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse release information")
}

func TestFetchLatestVersion_EmptyTagName(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":""}`)

	_, err := FetchLatestVersion(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no releases found")
}

func TestRunUpdate_AlreadyUpToDate(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.1.0"}`)
	runner := &fakeExecRunner{}
	paths := SummonPaths{
		BinaryPath: "/usr/local/bin/summon",
		BinaryDir:  "/usr/local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	result, err := RunUpdate("v0.1.0", paths, client, runner, &buf)
	require.NoError(t, err)
	assert.True(t, result.AlreadyUpToDate)
	assert.False(t, result.Updated)
	assert.Equal(t, "0.1.0", result.CurrentVersion)
	assert.Equal(t, "0.1.0", result.LatestVersion)
	assert.Empty(t, runner.commands, "should not execute installer when up to date")
}

func TestRunUpdate_SuccessfulUpdate(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.2.0"}`)

	// Serve installer script (platform-aware)
	installerURL := fmt.Sprintf("%s/v0.2.0/%s", defaultRawGitHub, installerScriptName())
	client.setJSON(installerURL, 200, `#!/bin/sh\necho "installed"`)

	runner := &fakeExecRunner{}
	paths := tempSummonPaths(t)
	var buf bytes.Buffer

	result, err := RunUpdate("v0.1.0", paths, client, runner, &buf)
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.False(t, result.AlreadyUpToDate)
	assert.Equal(t, "0.1.0", result.CurrentVersion)
	assert.Equal(t, "0.2.0", result.LatestVersion)
	assert.Len(t, runner.commands, 1, "installer should have been executed")
	assert.Contains(t, buf.String(), "updating summon v0.1.0 → v0.2.0")
}

func TestRunUpdate_DownloadFailure(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.2.0"}`)
	// No installer script served → will get 404

	runner := &fakeExecRunner{}
	paths := tempSummonPaths(t)
	var buf bytes.Buffer

	_, err := RunUpdate("v0.1.0", paths, client, runner, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
	assert.Contains(t, err.Error(), "not been modified")
	assert.Empty(t, runner.commands, "should not run installer on download failure")
}

func TestRunUpdate_InstallerFailure(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 200, `{"tag_name":"v0.2.0"}`)

	installerURL := fmt.Sprintf("%s/v0.2.0/%s", defaultRawGitHub, installerScriptName())
	client.setJSON(installerURL, 200, `#!/bin/sh\nexit 1`)

	runner := &fakeExecRunner{err: fmt.Errorf("exit status 1")}
	paths := tempSummonPaths(t)
	var buf bytes.Buffer

	_, err := RunUpdate("v0.1.0", paths, client, runner, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}

func TestRunUpdate_NetworkError(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.errors[defaultReleasesAPI] = fmt.Errorf("network unreachable")

	runner := &fakeExecRunner{}
	paths := SummonPaths{
		BinaryPath: "/home/user/.local/bin/summon",
		BinaryDir:  "/home/user/.local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	_, err := RunUpdate("v0.1.0", paths, client, runner, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for updates")
}

func TestFetchLatestVersion_ServerError(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()
	client.setJSON(defaultReleasesAPI, 500, `Internal Server Error`)

	_, err := FetchLatestVersion(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GitHub API returned status 500")
}

func TestPerformUpdate_DownloadNetworkError(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()

	installerURL := fmt.Sprintf("%s/v0.2.0/%s", defaultRawGitHub, installerScriptName())
	client.errors[installerURL] = fmt.Errorf("connection refused")

	runner := &fakeExecRunner{}
	paths := tempSummonPaths(t)
	var buf bytes.Buffer

	release := ReleaseInfo{TagName: "v0.2.0", Version: "0.2.0"}
	err := PerformUpdate(release, paths, client, runner, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
	assert.Contains(t, err.Error(), "not been modified")
	assert.Empty(t, runner.commands)
}

func TestPerformUpdate_InstallerReadError(t *testing.T) {
	clearURLEnvVars(t)
	client := newFakeHTTPClient()

	installerURL := fmt.Sprintf("%s/v0.2.0/%s", defaultRawGitHub, installerScriptName())
	client.responses[installerURL] = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&errReader{err: fmt.Errorf("read interrupted")}),
	}

	runner := &fakeExecRunner{}
	paths := tempSummonPaths(t)
	var buf bytes.Buffer

	release := ReleaseInfo{TagName: "v0.2.0", Version: "0.2.0"}
	err := PerformUpdate(release, paths, client, runner, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read installer script")
	assert.Empty(t, runner.commands)
}

// errReader is an io.Reader that always returns an error.
type errReader struct {
	err error
}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, e.err
}

func TestStripVersion(t *testing.T) {
	assert.Equal(t, "0.1.0", StripVersion("v0.1.0"))
	assert.Equal(t, "0.1.0", StripVersion("0.1.0"))
	assert.Equal(t, "dev", StripVersion("dev"))
}

func TestGetReleasesAPI_Default(t *testing.T) {
	t.Setenv("SUMMON_GITHUB_API", "")
	assert.Equal(t, defaultReleasesAPI, getReleasesAPI())
}

func TestGetReleasesAPI_EnvOverride(t *testing.T) {
	t.Setenv("SUMMON_GITHUB_API", "http://localhost:8080/repos/ai-summon/summon/releases/latest")
	assert.Equal(t, "http://localhost:8080/repos/ai-summon/summon/releases/latest", getReleasesAPI())
}

func TestGetRawGitHub_Default(t *testing.T) {
	t.Setenv("SUMMON_DOWNLOAD_BASE", "")
	assert.Equal(t, defaultRawGitHub, getRawGitHub())
}

func TestGetRawGitHub_EnvOverride(t *testing.T) {
	t.Setenv("SUMMON_DOWNLOAD_BASE", "http://localhost:8080")
	assert.Equal(t, "http://localhost:8080", getRawGitHub())
}

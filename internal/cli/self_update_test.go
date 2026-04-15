package cli

import (
	"bytes"
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

type fakeSelfUpdateHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func newFakeSelfUpdateHTTPClient() *fakeSelfUpdateHTTPClient {
	return &fakeSelfUpdateHTTPClient{
		responses: make(map[string]*http.Response),
		errors:    make(map[string]error),
	}
}

func (f *fakeSelfUpdateHTTPClient) Do(req *http.Request) (*http.Response, error) {
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

func (f *fakeSelfUpdateHTTPClient) setJSON(url string, statusCode int, body string) {
	f.responses[url] = &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type fakeSelfUpdateExecRunner struct {
	commands []string
	err      error
}

func (f *fakeSelfUpdateExecRunner) RunWithEnv(name string, args []string, env []string, stdout, stderr io.Writer) error {
	f.commands = append(f.commands, name+" "+strings.Join(args, " "))
	return f.err
}

func TestSelfUpdate_AlreadyUpToDate(t *testing.T) {
	t.Setenv("SUMMON_GITHUB_API", "")
	t.Setenv("SUMMON_DOWNLOAD_BASE", "")

	// Set version
	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	client := newFakeSelfUpdateHTTPClient()
	client.setJSON("https://api.github.com/repos/ai-summon/summon/releases/latest", 200, `{"tag_name":"v0.1.0"}`)

	var stdout bytes.Buffer
	deps := &selfUpdateDeps{
		httpClient: client,
		execRunner: &fakeSelfUpdateExecRunner{},
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	err := runSelfUpdate(deps)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "already up to date")
}

func TestSelfUpdate_SuccessfulUpdate(t *testing.T) {
	t.Setenv("SUMMON_GITHUB_API", "")
	t.Setenv("SUMMON_DOWNLOAD_BASE", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.0.13"
	defer func() { rootCmd.Version = oldVersion }()

	// Create a real temp binary so PrepareForUpdate succeeds on Windows
	tmpDir := t.TempDir()
	binaryName := "summon"
	if runtime.GOOS == "windows" {
		binaryName = "summon.exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0755))

	// Platform-aware installer URL
	scriptName := "install.sh"
	if runtime.GOOS == "windows" {
		scriptName = "install.ps1"
	}

	client := newFakeSelfUpdateHTTPClient()
	client.setJSON("https://api.github.com/repos/ai-summon/summon/releases/latest", 200, `{"tag_name":"v0.1.0"}`)
	client.setJSON("https://raw.githubusercontent.com/ai-summon/summon/v0.1.0/"+scriptName, 200, `#!/bin/sh\necho ok`)

	var stdout bytes.Buffer
	deps := &selfUpdateDeps{
		httpClient: client,
		execRunner: &fakeSelfUpdateExecRunner{},
		pathResolver: &fakePathResolver{
			executablePath: binaryPath,
			homeDir:        tmpDir,
		},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	err := runSelfUpdate(deps)
	require.NoError(t, err)
	output := stdout.String()
	assert.Contains(t, output, "updating summon v0.0.13 → v0.1.0")
	assert.Contains(t, output, "updated successfully")
}

func TestSelfUpdate_ErrorOutput(t *testing.T) {
	t.Setenv("SUMMON_GITHUB_API", "")
	t.Setenv("SUMMON_DOWNLOAD_BASE", "")

	oldVersion := rootCmd.Version
	rootCmd.Version = "0.1.0"
	defer func() { rootCmd.Version = oldVersion }()

	client := newFakeSelfUpdateHTTPClient()
	client.errors["https://api.github.com/repos/ai-summon/summon/releases/latest"] = assert.AnError

	var stdout bytes.Buffer
	deps := &selfUpdateDeps{
		httpClient: client,
		execRunner: &fakeSelfUpdateExecRunner{},
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	err := runSelfUpdate(deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for updates")
}

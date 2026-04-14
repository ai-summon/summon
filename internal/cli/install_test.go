package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helpers ---

type fakeRunner struct {
	commands  [][]string
	lookPaths map[string]string
	runFunc   func(name string, args ...string) ([]byte, error)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		lookPaths: map[string]string{
			"copilot": "/usr/local/bin/copilot",
		},
	}
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, append([]string{name}, args...))
	if f.runFunc != nil {
		return f.runFunc(name, args...)
	}
	// Default: return empty JSON list for "list" commands, success for others
	for _, a := range args {
		if a == "list" {
			return []byte("[]"), nil
		}
	}
	return nil, nil
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if path, ok := f.lookPaths[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("%s not found", name)
}

type fakeFetcher struct {
	manifests map[string]*manifest.Manifest
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{manifests: make(map[string]*manifest.Manifest)}
}

func (f *fakeFetcher) FetchManifest(source string) (*manifest.Manifest, error) {
	m, ok := f.manifests[source]
	if !ok {
		return nil, nil // No manifest — valid
	}
	return m, nil
}

func newTestDeps(runner *fakeRunner, fetcher *fakeFetcher, stdin string) *installDeps {
	return &installDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader(stdin),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}
}

// --- Tests ---

func TestInstall_BasicPlugin(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	deps := newTestDeps(runner, fetcher, "y\n")

	// Reset flags
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin")
	assert.Contains(t, out, "Installed 1 packages")
}

func TestInstall_WithDependencies(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()

	fetcher.manifests["https://github.com/owner/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Main plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin")
	assert.Contains(t, out, "dep-a")
	assert.Contains(t, out, "Installed 2 packages")
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"name":"my-plugin","source":"gh:owner/my-plugin"}]`), nil
			}
		}
		return nil, nil
	}

	fetcher := newFakeFetcher()
	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "already installed")
}

func TestInstall_NoManifest(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	// No manifest registered — fetcher returns nil

	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/no-manifest-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "no-manifest-plugin")
	assert.Contains(t, out, "Installed 1 packages")
}

func TestInstall_CycleDetection(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()

	fetcher.manifests["https://github.com/owner/a"] = &manifest.Manifest{
		Name:        "a",
		Description: "Plugin A",
		Dependencies: []string{"gh:owner/b"},
	}
	fetcher.manifests["https://github.com/owner/b"] = &manifest.Manifest{
		Name:        "b",
		Description: "Plugin B",
		Dependencies: []string{"gh:owner/a"},
	}

	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/a", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestInstall_SystemRequirementsMissing_YesMode(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()

	fetcher.manifests["https://github.com/owner/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin with sys reqs",
		SystemRequirements: []manifest.SystemRequirement{
			{Name: "nonexistent-binary-xyz"},
		},
	}

	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required system dependency missing")
}

func TestInstall_SystemRequirements_ForceMode(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()

	fetcher.manifests["https://github.com/owner/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin with sys reqs",
		SystemRequirements: []manifest.SystemRequirement{
			{Name: "nonexistent-binary-xyz"},
		},
	}

	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installForce = true
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err) // Force skips checks

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Installed 1 packages")
}

func TestInstall_NoCLIsDetected(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths = map[string]string{} // No CLIs

	fetcher := newFakeFetcher()
	deps := newTestDeps(runner, fetcher, "")
	installYes = true
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs detected")
}

// --- httpClientWrapper for testing ---

type fakeHTTPClient struct {
	responses map[string]*http.Response
}

func (f *fakeHTTPClient) Get(url string) (*http.Response, error) {
	if resp, ok := f.responses[url]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

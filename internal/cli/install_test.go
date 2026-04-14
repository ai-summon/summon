package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/marketplace"
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
			"claude": "/usr/local/bin/claude",
		},
	}
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, append([]string{name}, args...))
	if f.runFunc != nil {
		return f.runFunc(name, args...)
	}
	// Default: return empty list in format appropriate for each CLI
	for _, a := range args {
		if a == "list" {
			if name == "copilot" {
				return []byte("No plugins installed.\n"), nil
			}
			return []byte("[]"), nil // JSON for claude
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

// fakeIndexFetcher implements marketplace.IndexFetcher for testing.
type fakeIndexFetcher struct {
	indices map[string]marketplace.Index
}

func newFakeIndexFetcher() *fakeIndexFetcher {
	return &fakeIndexFetcher{indices: make(map[string]marketplace.Index)}
}

func (f *fakeIndexFetcher) FetchMarketplaceIndex(source string) (marketplace.Index, error) {
	idx, ok := f.indices[source]
	if !ok {
		return nil, fmt.Errorf("marketplace not found: %s", source)
	}
	return idx, nil
}

func (f *fakeIndexFetcher) LookupPackage(name string, marketplaceSource string) (*marketplace.PackageEntry, error) {
	idx, err := f.FetchMarketplaceIndex(marketplaceSource)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch marketplace index from %s: %w", marketplaceSource, err)
	}
	entry, ok := idx.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("package %q not found in marketplace %s", name, marketplaceSource)
	}
	return entry, nil
}

func newTestDeps(runner *fakeRunner, fetcher *fakeFetcher, stdin string) *installDeps {
	return &installDeps{
		runner:       runner,
		fetcher:      fetcher,
		indexFetcher: newFakeIndexFetcher(),
		stdin:        strings.NewReader(stdin),
		stdout:       &bytes.Buffer{},
		stderr:       &bytes.Buffer{},
	}
}

func newTestDepsWithIndex(runner *fakeRunner, fetcher *fakeFetcher, indexFetcher *fakeIndexFetcher, stdin string) *installDeps {
	return &installDeps{
		runner:       runner,
		fetcher:      fetcher,
		indexFetcher: indexFetcher,
		stdin:        strings.NewReader(stdin),
		stdout:       &bytes.Buffer{},
		stderr:       &bytes.Buffer{},
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
				return []byte(`[{"id":"my-plugin@marketplace","source":"gh:owner/my-plugin"}]`), nil
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

// --- Bare name (marketplace lookup) tests ---

func TestInstall_BareName_MarketplaceLookup(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	indexFetcher := newFakeIndexFetcher()

	// Register "superpowers" in the official marketplace
	indexFetcher.indices[marketplace.OfficialMarketplaceURL] = marketplace.Index{
		"superpowers": {Source: "gh:owner/superpowers", Description: "A superpower plugin"},
	}

	// The manifest fetcher should be queried with the resolved source
	fetcher.manifests["gh:owner/superpowers"] = nil

	deps := newTestDepsWithIndex(runner, fetcher, indexFetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("superpowers", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "superpowers")
	assert.Contains(t, out, "Installed 1 packages")

	// Verify the actual install command used the resolved source, not the bare name
	var installCmd []string
	for _, cmd := range runner.commands {
		if len(cmd) >= 4 && cmd[2] == "install" {
			installCmd = cmd
			break
		}
	}
	require.NotNil(t, installCmd, "expected an install command to be issued")
	assert.Equal(t, "gh:owner/superpowers", installCmd[3], "install should use resolved source, not bare name")
}

func TestInstall_BareName_NotFoundInMarketplace(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	indexFetcher := newFakeIndexFetcher()

	// Official marketplace exists but doesn't contain "nonexistent-pkg"
	indexFetcher.indices[marketplace.OfficialMarketplaceURL] = marketplace.Index{}

	deps := newTestDepsWithIndex(runner, fetcher, indexFetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("nonexistent-pkg", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in marketplace")
}

func TestInstall_BareName_WithDependencies(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	indexFetcher := newFakeIndexFetcher()

	// Register packages in the official marketplace
	indexFetcher.indices[marketplace.OfficialMarketplaceURL] = marketplace.Index{
		"superpowers":  {Source: "gh:owner/superpowers", Description: "Main plugin"},
		"helper-tools": {Source: "gh:owner/helper-tools", Description: "Helper"},
	}

	// superpowers depends on helper-tools (also a bare name)
	fetcher.manifests["gh:owner/superpowers"] = &manifest.Manifest{
		Name:         "superpowers",
		Description:  "Main plugin",
		Dependencies: []string{"helper-tools"},
	}

	deps := newTestDepsWithIndex(runner, fetcher, indexFetcher, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("superpowers", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "superpowers")
	assert.Contains(t, out, "helper-tools")
	assert.Contains(t, out, "Installed 2 packages")
}

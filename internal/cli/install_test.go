package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
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
	return nil, nil
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if path, ok := f.lookPaths[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("%s not found", name)
}

type fakeAdapter struct {
	name              string
	scopes            []platform.Scope
	installFunc       func(source string, scope platform.Scope) error
	uninstallFunc     func(name string, scope platform.Scope) error
	findDirFunc       func(name string, scope platform.Scope) (string, error)
	listInstalledFunc func(scope platform.Scope) ([]platform.InstalledPlugin, error)
	ensureMarketplaceFunc func(name, source string) error
	removeMarketplaceFunc func(name string) error
	listMarketplacesFunc  func() ([]platform.MarketplaceInfo, error)
	installedCmds     []string // track install calls
}

func newFakeAdapter(name string) *fakeAdapter {
	return &fakeAdapter{
		name:   name,
		scopes: []platform.Scope{platform.ScopeUser},
	}
}

func (f *fakeAdapter) Name() string                    { return f.name }
func (f *fakeAdapter) Detect() bool                    { return true }
func (f *fakeAdapter) SupportedScopes() []platform.Scope { return f.scopes }

func (f *fakeAdapter) Install(source string, scope platform.Scope) error {
	f.installedCmds = append(f.installedCmds, source)
	if f.installFunc != nil {
		return f.installFunc(source, scope)
	}
	return nil
}

func (f *fakeAdapter) Uninstall(name string, scope platform.Scope) error {
	if f.uninstallFunc != nil {
		return f.uninstallFunc(name, scope)
	}
	return nil
}
func (f *fakeAdapter) Update(name string, scope platform.Scope) error    { return nil }

func (f *fakeAdapter) ListInstalled(scope platform.Scope) ([]platform.InstalledPlugin, error) {
	if f.listInstalledFunc != nil {
		return f.listInstalledFunc(scope)
	}
	return nil, nil
}

func (f *fakeAdapter) EnsureMarketplace(name, source string) error {
	if f.ensureMarketplaceFunc != nil {
		return f.ensureMarketplaceFunc(name, source)
	}
	return nil
}

func (f *fakeAdapter) ListMarketplaces() ([]platform.MarketplaceInfo, error) {
	if f.listMarketplacesFunc != nil {
		return f.listMarketplacesFunc()
	}
	return nil, nil
}

func (f *fakeAdapter) RemoveMarketplace(name string) error {
	if f.removeMarketplaceFunc != nil {
		return f.removeMarketplaceFunc(name)
	}
	return nil
}

func (f *fakeAdapter) FindPluginDir(name string, scope platform.Scope) (string, error) {
	if f.findDirFunc != nil {
		return f.findDirFunc(name, scope)
	}
	return "", fmt.Errorf("no plugin dir for %s", name)
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
		return nil, nil
	}
	return m, nil
}

func newTestDeps(runner *fakeRunner, fetcher *fakeFetcher, adapters []platform.Adapter, stdin string) *installDeps {
	return &installDeps{
		runner:   runner,
		fetcher:  fetcher,
		adapters: adapters,
		stdin:    strings.NewReader(stdin),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}
}

// --- Tests ---

func TestInstall_BasicPlugin(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	deps := newTestDeps(runner, fetcher, []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin")
	assert.Contains(t, out, "Installed 1 packages")
	assert.Contains(t, adapter.installedCmds, "https://github.com/owner/my-plugin")
}

func TestInstall_WithDependencies(t *testing.T) {
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	// Configure FindPluginDir to return a path keyed by plugin name
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	// Register manifests keyed by the dir path that FindPluginDir returns
	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{adapter}, "")
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

func TestInstall_NoManifest(t *testing.T) {
	adapter := newFakeAdapter("claude")
	// FindPluginDir fails — no manifest discovery, but install succeeds
	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{adapter}, "")
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
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/a"] = &manifest.Manifest{
		Dependencies: []string{"gh:owner/b"},
	}
	fetcher.manifests["/fake/plugins/b"] = &manifest.Manifest{
		Dependencies: []string{"gh:owner/a"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	// Cycle should be detected but install continues (partial success)
	err := runInstall("gh:owner/a", deps)
	require.NoError(t, err) // cycles are warned, not fatal

	stderr := deps.stderr.(*bytes.Buffer).String()
	assert.Contains(t, stderr, "cycle")
}

func TestInstall_SystemRequirements_PostInstallWarning(t *testing.T) {
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		SystemRequirements: []manifest.SystemRequirement{
			{Name: "nonexistent-binary-xyz"},
		},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	// System requirements are post-install warnings now, not errors
	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	stderr := deps.stderr.(*bytes.Buffer).String()
	assert.Contains(t, stderr, "missing")

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Installed 1 packages")
}

func TestInstall_SystemRequirements_ForceMode(t *testing.T) {
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		SystemRequirements: []manifest.SystemRequirement{
			{Name: "nonexistent-binary-xyz"},
		},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{adapter}, "")
	installYes = true
	installForce = true
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Installed 1 packages")
	assert.NotContains(t, out, "System requirements")
}

func TestInstall_NoCLIsDetected(t *testing.T) {
	// Empty adapters list simulates no CLIs detected
	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{}, "")
	installYes = true
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs detected")
}

// --- Bare name (native delegation) tests ---

func TestInstall_BareName_NativeDelegation(t *testing.T) {
	adapter := newFakeAdapter("claude")
	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("superpowers", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "superpowers")
	assert.Contains(t, out, "Installed 1 packages")

	// Verify the install command used name@summon-marketplace
	require.Len(t, adapter.installedCmds, 1)
	assert.Equal(t, "superpowers@summon-marketplace", adapter.installedCmds[0])
}

func TestInstall_BareName_WithDependencies_NativeDelegation(t *testing.T) {
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/superpowers"] = &manifest.Manifest{
		Dependencies: []string{"helper-tools"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{adapter}, "")
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

func TestInstall_MultiAdapter(t *testing.T) {
	fetcher := newFakeFetcher()
	claude := newFakeAdapter("claude")
	copilot := newFakeAdapter("copilot")

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{claude, copilot}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Installed 1 packages")

	// Both adapters should have been called
	assert.Len(t, claude.installedCmds, 1)
	assert.Len(t, copilot.installedCmds, 1)
}

func TestInstall_PartialFailure(t *testing.T) {
	fetcher := newFakeFetcher()
	claude := newFakeAdapter("claude")
	copilot := newFakeAdapter("copilot")

	copilot.installFunc = func(source string, scope platform.Scope) error {
		return fmt.Errorf("copilot install failed")
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{claude, copilot}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err) // partial failure is not a fatal error

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "0 of 1") // partial: package had one CLI fail
	assert.Contains(t, out, "✗")
}

// --- renderSummary unit tests ---

func TestRenderSummary_AllSuccess(t *testing.T) {
	var buf bytes.Buffer
	summary := &InstallSummary{
		CLIs:           []string{"claude"},
		TotalInstalled: 2,
		Results: []InstallResult{
			{PackageName: "pkg-a", CLIResults: map[string]error{"claude": nil}},
			{PackageName: "pkg-b", CLIResults: map[string]error{"claude": nil}},
		},
	}
	renderSummary(summary, &buf)
	out := buf.String()
	assert.Contains(t, out, "Installed 2 packages")
	assert.Contains(t, out, "✓")
	assert.Contains(t, out, "pkg-a")
	assert.Contains(t, out, "pkg-b")
	assert.NotContains(t, out, "✗")
}

func TestRenderSummary_Mixed(t *testing.T) {
	var buf bytes.Buffer
	summary := &InstallSummary{
		CLIs:           []string{"claude", "copilot"},
		TotalInstalled: 0,
		TotalFailed:    1,
		Results: []InstallResult{
			{
				PackageName: "pkg-x",
				CLIResults: map[string]error{
					"claude":  nil,
					"copilot": fmt.Errorf("failed"),
				},
			},
		},
	}
	renderSummary(summary, &buf)
	out := buf.String()
	assert.Contains(t, out, "0 of 1")
	assert.Contains(t, out, "✓")
	assert.Contains(t, out, "✗")
}

func TestRenderSummary_Empty(t *testing.T) {
	var buf bytes.Buffer
	summary := &InstallSummary{CLIs: []string{"claude"}}
	renderSummary(summary, &buf)
	assert.Empty(t, buf.String())
}

// --- Progress message tests ---

func TestInstall_ProgressMessages_PerAdapterInstall(t *testing.T) {
	adapter := newFakeAdapter("claude")
	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("superpowers", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	// No marketplace check messages in stdout
	assert.NotContains(t, out, "Checking marketplace")
	assert.NotContains(t, out, "marketplace ready")
	// Platform header
	assert.Contains(t, out, "claude:")
	// Per-adapter install feedback (no "on {platform}" suffix)
	assert.Contains(t, out, "  Installing superpowers...")
	assert.Contains(t, out, "✓ superpowers installed")
	assert.NotContains(t, out, "on claude")
}

func TestInstall_ProgressMessages_DependencyInstall(t *testing.T) {
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}
	fetcher.manifests["/fake/plugins/main-pkg"] = &manifest.Manifest{
		Dependencies: []string{"dep-a"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("main-pkg", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	// Platform header
	assert.Contains(t, out, "claude:")
	// Dependency install with indentation (no "on {platform}" suffix)
	assert.Contains(t, out, "Installing dependency dep-a...")
	assert.Contains(t, out, "✓ dep-a installed")
	assert.NotContains(t, out, "on claude")
}

func TestInstall_ProgressMessages_NoMarketplaceForGitHub(t *testing.T) {
	adapter := newFakeAdapter("claude")
	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	// No marketplace check for GitHub shorthand
	assert.NotContains(t, out, "Checking marketplace")
	// Platform header and install feedback (no "on {platform}" suffix)
	assert.Contains(t, out, "claude:")
	assert.Contains(t, out, "  Installing my-plugin...")
	assert.Contains(t, out, "✓ my-plugin installed")
	assert.NotContains(t, out, "on claude")
}

// --- Platform-first output tests ---

func TestInstall_PlatformFirstOutput_MultiAdapter(t *testing.T) {
	fetcher := newFakeFetcher()
	claude := newFakeAdapter("claude")
	copilot := newFakeAdapter("copilot")

	claude.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}
	copilot.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/wingman"] = &manifest.Manifest{
		Dependencies: []string{"speckit"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{claude, copilot}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("wingman", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()

	// Verify platform-first ordering: claude section appears before copilot section
	claudeIdx := strings.Index(out, "claude:")
	copilotIdx := strings.Index(out, "copilot:")
	require.NotEqual(t, -1, claudeIdx, "claude header not found")
	require.NotEqual(t, -1, copilotIdx, "copilot header not found")
	assert.Less(t, claudeIdx, copilotIdx, "claude section should appear before copilot section")

	// Verify both platforms install wingman and speckit
	assert.Contains(t, out, "  Installing wingman...")
	assert.Contains(t, out, "✓ wingman installed")
	assert.Contains(t, out, "Installing dependency speckit...")
	assert.Contains(t, out, "✓ speckit installed")

	// Verify no "on {platform}" suffix in install messages
	assert.NotContains(t, out, "on claude")
	assert.NotContains(t, out, "on copilot")

	// Both adapters should have installed both packages
	assert.Len(t, claude.installedCmds, 2)
	assert.Len(t, copilot.installedCmds, 2)

	// Summary
	assert.Contains(t, out, "Installed 2 packages")
}

func TestInstall_PlatformFirst_MainFailSkipsDeps(t *testing.T) {
	fetcher := newFakeFetcher()
	claude := newFakeAdapter("claude")
	copilot := newFakeAdapter("copilot")

	// copilot fails to install main package
	copilot.installFunc = func(source string, scope platform.Scope) error {
		return fmt.Errorf("copilot install failed")
	}

	claude.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/wingman"] = &manifest.Manifest{
		Dependencies: []string{"speckit"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{claude, copilot}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("wingman", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()

	// Claude succeeds with deps
	assert.Contains(t, out, "claude:")
	assert.Contains(t, out, "✓ wingman installed")
	assert.Contains(t, out, "✓ speckit installed")

	// Copilot fails main → deps skipped
	assert.Contains(t, out, "copilot:")
	assert.Contains(t, out, "✗ wingman failed")

	// Claude installed both, copilot only attempted main
	assert.Len(t, claude.installedCmds, 2)
	assert.Len(t, copilot.installedCmds, 1)
}

func TestInstall_PlatformFirst_DepErrorDoesNotAbortNextAdapter(t *testing.T) {
	fetcher := newFakeFetcher()
	claude := newFakeAdapter("claude")
	copilot := newFakeAdapter("copilot")

	// claude fails dep installs, copilot succeeds
	callCount := 0
	claude.installFunc = func(source string, scope platform.Scope) error {
		callCount++
		if callCount > 1 {
			return fmt.Errorf("dep install failed on claude")
		}
		return nil
	}

	claude.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}
	copilot.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher.manifests["/fake/plugins/wingman"] = &manifest.Manifest{
		Dependencies: []string{"speckit"},
	}

	deps := newTestDeps(newFakeRunner(), fetcher, []platform.Adapter{claude, copilot}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("wingman", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()

	// Both platform headers present
	assert.Contains(t, out, "claude:")
	assert.Contains(t, out, "copilot:")

	// Copilot still installed both (dep error on claude didn't abort copilot)
	assert.Len(t, copilot.installedCmds, 2)
}
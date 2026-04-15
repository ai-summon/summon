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

func TestUninstall_NoDependents(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin uninstalled")
}

func TestUninstall_WithDependents(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
			{Name: "dep-a", Source: "dep-a@marketplace", Platform: "claude"},
		}, nil
	}
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Main plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{adapter},
		stdin:    strings.NewReader("y\n"),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = false
	installScope = "user"
	targetFlag = ""

	err := runUninstall("dep-a", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "⚠️")
	assert.Contains(t, out, "my-plugin")
	assert.Contains(t, out, "dep-a uninstalled")
}

func TestUninstall_NotInstalled(t *testing.T) {
	adapter := newFakeAdapter("claude")

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("nonexistent", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestUninstall_YesSkipsConfirmation(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
			{Name: "dep-a", Source: "dep-a@marketplace", Platform: "claude"},
		}, nil
	}
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Main plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{adapter},
		stdin:    strings.NewReader(""), // no input
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true // --yes flag
	installScope = "user"
	targetFlag = ""

	err := runUninstall("dep-a", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "dep-a uninstalled")
}

func TestUninstall_NoCLIs(t *testing.T) {
	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{}, // empty = no CLIs
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs")
}

func TestUninstall_PartialFailure(t *testing.T) {
	copilot := newFakeAdapter("copilot")
	copilot.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "copilot"},
		}, nil
	}
	claude := newFakeAdapter("claude")
	claude.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
		}, nil
	}
	claude.uninstallFunc = func(name string, scope platform.Scope) error {
		return fmt.Errorf("plugin not found")
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{copilot, claude},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.Error(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "✓ my-plugin uninstalled (copilot)")
	assert.Contains(t, out, "✗ failed to uninstall my-plugin from claude")
	assert.Contains(t, out, "Partially uninstalled")
	assert.Contains(t, err.Error(), "1 platform(s)")
}

func TestUninstall_AllPlatformsFail(t *testing.T) {
	failUninstall := func(name string, scope platform.Scope) error {
		return fmt.Errorf("exit status 1")
	}
	copilot := newFakeAdapter("copilot")
	copilot.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "copilot"},
		}, nil
	}
	copilot.uninstallFunc = failUninstall
	claude := newFakeAdapter("claude")
	claude.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
		}, nil
	}
	claude.uninstallFunc = failUninstall

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{copilot, claude},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 platform(s)")

	out := deps.stdout.(*bytes.Buffer).String()
	assert.NotContains(t, out, "Partially uninstalled")
}

func TestUninstall_ProjectScope(t *testing.T) {
	var uninstallCalls []struct {
		name  string
		scope platform.Scope
	}
	claude := newFakeAdapter("claude")
	claude.scopes = []platform.Scope{platform.ScopeUser, platform.ScopeProject}
	claude.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude", Scope: "project"},
		}, nil
	}
	claude.uninstallFunc = func(name string, scope platform.Scope) error {
		uninstallCalls = append(uninstallCalls, struct {
			name  string
			scope platform.Scope
		}{name, scope})
		return nil
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{claude},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin uninstalled")

	// Verify uninstall was called with project scope (from plugin's reported scope)
	require.Len(t, uninstallCalls, 1)
	assert.Equal(t, platform.ScopeProject, uninstallCalls[0].scope)
}

func TestUninstall_DeduplicatesReverseDeps(t *testing.T) {
	// Same plugin listed on two adapters — should only warn once
	copilot := newFakeAdapter("copilot")
	copilot.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "parent", Platform: "copilot"},
			{Name: "child", Platform: "copilot"},
		}, nil
	}
	copilot.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/" + name, nil
	}
	claude := newFakeAdapter("claude")
	claude.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "parent", Platform: "claude"},
			{Name: "child", Platform: "claude"},
		}, nil
	}
	claude.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/" + name, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/parent"] = &manifest.Manifest{
		Name:         "parent",
		Dependencies: []string{"child"},
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{copilot, claude},
		stdin:    strings.NewReader("y\n"),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = false
	installScope = "user"
	targetFlag = ""

	err := runUninstall("child", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	// "parent" should appear exactly once in the warning
	assert.Equal(t, 1, strings.Count(out, "• parent"))
}

// Suppress unused import warning
var _ = fmt.Sprintf

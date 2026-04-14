package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUninstall_NoDependents(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"id":"my-plugin@marketplace"}]`), nil
			}
		}
		return nil, nil
	}

	fetcher := newFakeFetcher()
	deps := &uninstallDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
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
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[
					{"id":"my-plugin@marketplace"},
					{"id":"dep-a@marketplace"}
				]`), nil
			}
		}
		return nil, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["my-plugin@marketplace"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Main plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader("y\n"),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
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
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte("[]"), nil
			}
		}
		return nil, nil
	}

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("nonexistent", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestUninstall_YesSkipsConfirmation(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[
					{"id":"my-plugin@marketplace"},
					{"id":"dep-a@marketplace"}
				]`), nil
			}
		}
		return nil, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["my-plugin@marketplace"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Main plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader(""), // no input
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
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
	runner := newFakeRunner()
	runner.lookPaths = map[string]string{} // No CLIs

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs")
}

func TestUninstall_PartialFailure(t *testing.T) {
	// Simulates: copilot uninstall succeeds, claude uninstall fails
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				if name == "copilot" {
					return []byte("  • my-plugin (v1.0.0)\n"), nil
				}
				return []byte(`[{"id":"my-plugin@marketplace"}]`), nil
			}
			if a == "uninstall" {
				if name == "claude" {
					return []byte("Error: plugin not found"), fmt.Errorf("exit status 1")
				}
				return nil, nil // copilot succeeds
			}
		}
		return nil, nil
	}

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.Error(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	// Copilot should show success
	assert.Contains(t, out, "✓ my-plugin uninstalled (copilot)")
	// Claude should show failure
	assert.Contains(t, out, "✗ failed to uninstall my-plugin from claude")
	// Should report partial uninstall
	assert.Contains(t, out, "Partially uninstalled")
	// Error should report the failure count
	assert.Contains(t, err.Error(), "1 platform(s)")
}

func TestUninstall_AllPlatformsFail(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				if name == "copilot" {
					return []byte("  • my-plugin (v1.0.0)\n"), nil
				}
				return []byte(`[{"id":"my-plugin@marketplace"}]`), nil
			}
			if a == "uninstall" {
				return []byte("some error"), fmt.Errorf("exit status 1")
			}
		}
		return nil, nil
	}

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 platform(s)")

	out := deps.stdout.(*bytes.Buffer).String()
	// Should NOT show partial uninstall message (none succeeded)
	assert.NotContains(t, out, "Partially uninstalled")
}

func TestUninstall_ProjectScope(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				if name == "copilot" {
					return []byte("  • my-plugin (v1.0.0)\n"), nil
				}
				return []byte(`[{"id":"my-plugin@marketplace","scope":"project"}]`), nil
			}
		}
		return nil, nil
	}

	deps := &uninstallDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin uninstalled")

	// Verify claude's uninstall was called with --scope project
	var foundClaudeUninstall bool
	for _, cmd := range runner.commands {
		if len(cmd) >= 4 && cmd[0] == "claude" && cmd[2] == "uninstall" {
			foundClaudeUninstall = true
			assert.Contains(t, cmd, "--scope", "claude uninstall should include --scope flag")
			assert.Contains(t, cmd, "project", "claude uninstall should use project scope")
		}
	}
	assert.True(t, foundClaudeUninstall, "claude uninstall command should have been called")
}

// Suppress unused import warning
var _ = fmt.Sprintf

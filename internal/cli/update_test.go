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

func TestUpdate_BasicUpdate(t *testing.T) {
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
	deps := &updateDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin updated")
}

func TestUpdate_WithNewDeps(t *testing.T) {
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
	fetcher.manifests["my-plugin@marketplace"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin",
		Dependencies: []string{"gh:owner/new-dep"},
	}

	deps := &updateDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "New dependency")
}

func TestUpdate_NotInstalled(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte("[]"), nil
			}
		}
		return nil, nil
	}

	deps := &updateDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdate("nonexistent", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestUpdateAll(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"id":"plugin-a@marketplace"},{"id":"plugin-b@marketplace"}]`), nil
			}
		}
		return nil, nil
	}

	fetcher := newFakeFetcher()
	deps := &updateDeps{
		runner:  runner,
		fetcher: fetcher,
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdateAll(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Updating all")
}

func TestUpdate_ProjectScope(t *testing.T) {
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

	deps := &updateDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin updated")

	// Verify claude's update was called with --scope project
	var foundClaudeUpdate bool
	for _, cmd := range runner.commands {
		if len(cmd) >= 4 && cmd[0] == "claude" && cmd[2] == "update" {
			foundClaudeUpdate = true
			assert.Contains(t, cmd, "--scope", "claude update should include --scope flag")
			assert.Contains(t, cmd, "project", "claude update should use project scope")
		}
	}
	assert.True(t, foundClaudeUpdate, "claude update command should have been called")
}

func TestUpdate_PartialFailure(t *testing.T) {
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
			if a == "update" {
				if name == "claude" {
					return []byte("update error"), fmt.Errorf("exit status 1")
				}
				return nil, nil // copilot succeeds
			}
		}
		return nil, nil
	}

	deps := &updateDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdin:   strings.NewReader(""),
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	// Should NOT return error since copilot succeeded
	err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin updated (copilot)")

	errOut := deps.stderr.(*bytes.Buffer).String()
	assert.Contains(t, errOut, "update failed on claude")
}

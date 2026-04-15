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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	_, err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin:")
	// No version info available → fallback to "updated"
	assert.Contains(t, out, "updated")
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	_, err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "new dependency")
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	_, err := runUpdate("nonexistent", deps)
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdateAll(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Updating")
	assert.Contains(t, out, "plugins")
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	_, err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin:")
	// Copilot has version → shows "up to date"; claude has no version → shows "updated"
	assert.Contains(t, out, "up to date")

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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	// Should NOT return error since copilot succeeded
	_, err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin:")
	assert.Contains(t, out, "copilot")
	assert.Contains(t, out, "up to date")
	assert.Contains(t, out, "claude")
	assert.Contains(t, out, "failed")
}

func TestUpdate_ClaudeUsesSourceIdentifier(t *testing.T) {
	// Claude CLI requires full source (name@marketplace), not bare name
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				if name == "copilot" {
					return []byte("  • speckit (v0.7.0)\n"), nil
				}
				return []byte(`[{"id":"speckit@summon-marketplace","version":"0.7.0"}]`), nil
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	_, err := runUpdate("speckit", deps)
	require.NoError(t, err)

	// Verify claude received the full source identifier, not bare name
	var claudeUpdateArgs []string
	var copilotUpdateArgs []string
	for _, cmd := range runner.commands {
		if len(cmd) >= 4 && cmd[2] == "update" {
			if cmd[0] == "claude" {
				claudeUpdateArgs = cmd
			} else if cmd[0] == "copilot" {
				copilotUpdateArgs = cmd
			}
		}
	}
	require.NotNil(t, claudeUpdateArgs, "claude update should have been called")
	assert.Equal(t, "speckit@summon-marketplace", claudeUpdateArgs[3],
		"claude update should use full source identifier")

	require.NotNil(t, copilotUpdateArgs, "copilot update should have been called")
	assert.Equal(t, "speckit", copilotUpdateArgs[3],
		"copilot update should use bare plugin name")
}

func TestUpdate_SkipsAdaptersWhereNotInstalled(t *testing.T) {
	// Plugin installed only on copilot should not attempt update on claude
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				if name == "copilot" {
					return []byte("  • copilot-only (v1.0.0)\n"), nil
				}
				return []byte(`[]`), nil // not installed on claude
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	_, err := runUpdate("copilot-only", deps)
	require.NoError(t, err)

	// Verify claude was NOT called for update
	for _, cmd := range runner.commands {
		if cmd[0] == "claude" && len(cmd) >= 3 && cmd[2] == "update" {
			t.Fatal("claude update should not be called for a plugin not installed on claude")
		}
	}
}

func TestUpdate_UpToDate(t *testing.T) {
	// Version unchanged after update → "up to date"
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"id":"my-plugin@marketplace","version":"1.2.0"}]`), nil
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	result, err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "up to date (v1.2.0)")
	assert.Contains(t, out, "–")
	assert.NotContains(t, out, "✓")
	assert.Equal(t, 0, result.updated)
	assert.Equal(t, 1, result.upToDate)
}

func TestUpdate_VersionChanged(t *testing.T) {
	// Version changes after update → "v1.0.0 → v1.1.0"
	listCallCount := 0
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				listCallCount++
				if listCallCount <= 1 {
					return []byte(`[{"id":"my-plugin@marketplace","version":"1.0.0"}]`), nil
				}
				return []byte(`[{"id":"my-plugin@marketplace","version":"1.1.0"}]`), nil
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	result, err := runUpdate("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "v1.0.0 → v1.1.0")
	assert.Contains(t, out, "✓")
	assert.Equal(t, 1, result.updated)
	assert.Equal(t, 0, result.upToDate)
}

func TestUpdateAll_Summary(t *testing.T) {
	// Two plugins: one up to date, one with unknown version
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"id":"plugin-a@marketplace","version":"1.0.0"},{"id":"plugin-b@marketplace"}]`), nil
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
		noColor: true,
	}

	installScope = "user"
	targetFlag = ""

	err := runUpdateAll(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "Updating")
	// plugin-a has version → "up to date", plugin-b has no version → "updated"
	assert.Contains(t, out, "1 updated")
	assert.Contains(t, out, "1 up to date")
}

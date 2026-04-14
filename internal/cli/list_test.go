package cli

import (
	"bytes"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_WithPlugins(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[
					{"id":"my-plugin@marketplace"},
					{"id":"other-plugin@marketplace"}
				]`), nil
			}
		}
		return nil, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["my-plugin@marketplace"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin",
		Dependencies: []string{"gh:owner/other-plugin"},
	}

	deps := &listDeps{
		runner:  runner,
		fetcher: fetcher,
		stdout:  &bytes.Buffer{},
	}

	listJSON = false
	installScope = "user"
	targetFlag = ""

	err := runList(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "my-plugin")
	assert.Contains(t, out, "other-plugin")
	assert.Contains(t, out, "└──")
}

func TestList_NoPlugins(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte("[]"), nil
			}
		}
		return nil, nil
	}

	deps := &listDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdout:  &bytes.Buffer{},
	}

	listJSON = false
	installScope = "user"
	targetFlag = ""

	err := runList(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "No plugins installed")
}

func TestList_JSONOutput(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"id":"my-plugin@marketplace"}]`), nil
			}
		}
		return nil, nil
	}

	deps := &listDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdout:  &bytes.Buffer{},
	}

	listJSON = true
	installScope = "user"
	targetFlag = ""

	err := runList(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "\"claude\"")
	assert.Contains(t, out, "\"my-plugin\"")
}

func TestList_TargetFilter(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"id":"my-plugin@marketplace"}]`), nil
			}
		}
		return nil, nil
	}

	deps := &listDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdout:  &bytes.Buffer{},
	}

	listJSON = false
	installScope = "user"
	targetFlag = "claude"

	err := runList(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "claude")
}

package cli

import (
	"bytes"
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

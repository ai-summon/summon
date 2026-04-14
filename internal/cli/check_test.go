package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_AllDepsSatisfied(t *testing.T) {
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
		Description: "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := &checkDeps{
		runner:  runner,
		fetcher: fetcher,
		stdout:  &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err)
}

func TestCheck_RequiredMissing(t *testing.T) {
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
		Dependencies: []string{"gh:owner/missing-plugin"},
	}

	deps := &checkDeps{
		runner:  runner,
		fetcher: fetcher,
		stdout:  &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

func TestCheck_SinglePlugin(t *testing.T) {
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
	deps := &checkDeps{
		runner:  runner,
		fetcher: fetcher,
		stdout:  &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("my-plugin", deps)
	require.NoError(t, err)
}

func TestCheck_NotInstalled(t *testing.T) {
	runner := newFakeRunner()
	runner.runFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte("[]"), nil
			}
		}
		return nil, nil
	}

	deps := &checkDeps{
		runner:  runner,
		fetcher: newFakeFetcher(),
		stdout:  &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("nonexistent", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestCheck_JSONOutput(t *testing.T) {
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
	deps := &checkDeps{
		runner:  runner,
		fetcher: fetcher,
		stdout:  &bytes.Buffer{},
	}

	checkJSON = true
	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.True(t, strings.Contains(out, "\"name\""))
}

func TestCheck_RecommendedMissing_StillOK(t *testing.T) {
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
		SystemRequirements: []manifest.SystemRequirement{
			{Name: "nonexistent-binary-test-xyz", Optional: true, Reason: "Only for testing"},
		},
	}

	deps := &checkDeps{
		runner:  runner,
		fetcher: fetcher,
		stdout:  &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err) // Recommended missing = still OK
}

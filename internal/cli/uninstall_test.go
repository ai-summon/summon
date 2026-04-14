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

// Suppress unused import warning
var _ = fmt.Sprintf

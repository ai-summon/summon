package cli

import (
	"bytes"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_WithPlugins(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
			{Name: "other-plugin", Source: "other-plugin@marketplace", Platform: "claude"},
		}, nil
	}
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin",
		Dependencies: []string{"gh:owner/other-plugin"},
	}

	deps := &listDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
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
	adapter := newFakeAdapter("claude")

	deps := &listDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
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
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}

	deps := &listDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
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
	claude := newFakeAdapter("claude")
	claude.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
		}, nil
	}
	copilot := newFakeAdapter("copilot")
	copilot.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "copilot-only", Platform: "copilot"},
		}, nil
	}

	deps := &listDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{copilot, claude},
		stdout:   &bytes.Buffer{},
	}

	listJSON = false
	installScope = "user"
	targetFlag = "claude"

	err := runList(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, "claude")
	assert.Contains(t, out, "my-plugin")
	assert.NotContains(t, out, "copilot-only")
}

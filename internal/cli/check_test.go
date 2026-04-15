package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_AllDepsSatisfied(t *testing.T) {
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
		Description: "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err)
}

func TestCheck_RequiredMissing(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin",
		Dependencies: []string{"gh:owner/missing-plugin"},
	}

	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

func TestCheck_SinglePlugin(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}

	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("my-plugin", deps)
	require.NoError(t, err)
}

func TestCheck_NotInstalled(t *testing.T) {
	adapter := newFakeAdapter("claude")

	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
	}

	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("nonexistent", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestCheck_JSONOutput(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}

	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
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
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/plugins/my-plugin"] = &manifest.Manifest{
		Name:        "my-plugin",
		Description: "Plugin",
		SystemRequirements: []manifest.SystemRequirement{
			{Name: "nonexistent-binary-test-xyz", Optional: true, Reason: "Only for testing"},
		},
	}

	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{adapter},
		stdout:   &bytes.Buffer{},
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err) // Recommended missing = still OK
}

package cli

import (
	"bytes"
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
		noColor:  true,
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
		noColor:  true,
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
		noColor:  true,
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
		noColor:  true,
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
		noColor:  true,
	}

	checkJSON = true
	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, `"claude"`)
	assert.Contains(t, out, `"name"`)
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
		noColor:  true,
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err) // Recommended missing = still OK
}

// Regression: dep installed on one platform but missing on another must be detected.
func TestCheck_CrossPlatform_DepMissingOnOne(t *testing.T) {
	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/copilot/my-plugin"] = &manifest.Manifest{
		Name:         "my-plugin",
		Description:  "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}
	fetcher.manifests["/fake/claude/my-plugin"] = &manifest.Manifest{
		Name:         "my-plugin",
		Description:  "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	copilotAdapter := newFakeAdapter("copilot")
	copilotAdapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "copilot"},
			{Name: "dep-a", Platform: "copilot"},
		}, nil
	}
	copilotAdapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/copilot/" + name, nil
	}

	claudeAdapter := newFakeAdapter("claude")
	claudeAdapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
			// dep-a is NOT installed on claude
		}, nil
	}
	claudeAdapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/claude/" + name, nil
	}

	var buf bytes.Buffer
	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{copilotAdapter, claudeAdapter},
		stdout:   &buf,
		noColor:  true,
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.Error(t, err, "should fail because dep-a is missing on claude")
	assert.Contains(t, err.Error(), "health check failed")

	out := buf.String()
	assert.Contains(t, out, "copilot:")
	assert.Contains(t, out, "claude:")
	assert.Contains(t, out, "✗ dep-a")
	assert.Contains(t, out, "required")
}

func TestCheck_SinglePlugin_MultiPlatform(t *testing.T) {
	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/copilot/my-plugin"] = &manifest.Manifest{
		Name:         "my-plugin",
		Description:  "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}
	fetcher.manifests["/fake/claude/my-plugin"] = &manifest.Manifest{
		Name:         "my-plugin",
		Description:  "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	copilotAdapter := newFakeAdapter("copilot")
	copilotAdapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "copilot"},
			{Name: "dep-a", Platform: "copilot"},
		}, nil
	}
	copilotAdapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/copilot/" + name, nil
	}

	claudeAdapter := newFakeAdapter("claude")
	claudeAdapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
			// dep-a missing on claude
		}, nil
	}
	claudeAdapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/claude/" + name, nil
	}

	var buf bytes.Buffer
	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{copilotAdapter, claudeAdapter},
		stdout:   &buf,
		noColor:  true,
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckSingle("my-plugin", deps)
	require.Error(t, err, "should fail because dep-a missing on claude")
	assert.Contains(t, err.Error(), "health check failed for my-plugin")

	out := buf.String()
	assert.Contains(t, out, "copilot:")
	assert.Contains(t, out, "claude:")
	assert.Contains(t, out, "✗ dep-a")
	assert.Contains(t, out, "required")
}

func TestCheck_CrossPlatform_JSON(t *testing.T) {
	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/copilot/my-plugin"] = &manifest.Manifest{
		Name:         "my-plugin",
		Description:  "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}
	fetcher.manifests["/fake/claude/my-plugin"] = &manifest.Manifest{
		Name:         "my-plugin",
		Description:  "Plugin",
		Dependencies: []string{"gh:owner/dep-a"},
	}

	copilotAdapter := newFakeAdapter("copilot")
	copilotAdapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "copilot"},
			{Name: "dep-a", Platform: "copilot"},
		}, nil
	}
	copilotAdapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/copilot/" + name, nil
	}

	claudeAdapter := newFakeAdapter("claude")
	claudeAdapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
		}, nil
	}
	claudeAdapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/claude/" + name, nil
	}

	var buf bytes.Buffer
	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  fetcher,
		adapters: []platform.Adapter{copilotAdapter, claudeAdapter},
		stdout:   &buf,
		noColor:  true,
	}

	checkJSON = true
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.Error(t, err)

	out := buf.String()
	assert.Contains(t, out, `"copilot"`)
	assert.Contains(t, out, `"claude"`)
	assert.Contains(t, out, `"installed": true`)
	assert.Contains(t, out, `"installed": false`)
}

func TestCheck_PlatformHeaderInOutput(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Platform: "claude"},
		}, nil
	}

	var buf bytes.Buffer
	deps := &checkDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdout:   &buf,
		noColor:  true,
	}

	checkJSON = false
	installScope = "user"
	targetFlag = ""

	err := runCheckAll(deps)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "claude:")
	assert.Contains(t, out, "✓ my-plugin")
	assert.Contains(t, out, "no dependencies")
}

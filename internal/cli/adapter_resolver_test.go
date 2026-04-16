package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/config"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func writeConfig(t *testing.T, dir string, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, config.Save(path, cfg))
	return path
}

func TestResolveAdapters_TestInjection(t *testing.T) {
	a1 := newFakeAdapter("claude")
	a2 := newFakeAdapter("copilot")
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		adapters: []platform.Adapter{a1, a2},
		stderr:   stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestResolveAdapters_TestInjectionWithTarget(t *testing.T) {
	a1 := newFakeAdapter("claude")
	a2 := newFakeAdapter("copilot")
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		adapters: []platform.Adapter{a1, a2},
		target:   "claude",
		stderr:   stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "claude", result[0].Name())
}

func TestResolveAdapters_NoConfigBootstraps(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Verify config was saved
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.True(t, cfg.HasPlatforms())
	assert.Equal(t, boolPtr(true), cfg.Platforms.Copilot)
	assert.Equal(t, boolPtr(true), cfg.Platforms.Claude)
}

func TestResolveAdapters_ConfigFiltersDisabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(true),
			Claude:  boolPtr(false),
		},
	})
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "copilot", result[0].Name())
}

func TestResolveAdapters_TargetOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(true),
			Claude:  boolPtr(false),
		},
	})
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		target:     "claude",
		stderr:     stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "claude", result[0].Name())
	assert.Contains(t, stderr.String(), "using disabled platform claude")
}

func TestResolveAdapters_EnabledButNotInstalled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(true),
			Claude:  boolPtr(true),
		},
	})
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	// claude NOT in lookPaths
	delete(runner.lookPaths, "claude")
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "copilot", result[0].Name())
	assert.Contains(t, stderr.String(), "claude is enabled but not installed")
}

func TestResolveAdapters_AllDisabledError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(false),
			Claude:  boolPtr(false),
		},
	})
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
	}
	_, err := resolveEnabledAdapters(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no enabled platforms")
}

func TestResolveAdapters_NoCLIsDetectedError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	runner := newFakeRunner()
	delete(runner.lookPaths, "claude")
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
	}
	_, err := resolveEnabledAdapters(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs detected")
}

func TestResolveAdapters_UnreadableConfigFallsBack(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	// Write invalid YAML
	require.NoError(t, os.WriteFile(cfgPath, []byte(":::invalid"), 0o644))

	runner := newFakeRunner()
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "claude", result[0].Name())
	assert.Contains(t, stderr.String(), "could not read config")
}

func TestResolveAdapters_BootstrapSaveFailureNonFatal(t *testing.T) {
	// Use an injectable save function that always fails — avoids platform-
	// specific filesystem permission tricks that don't work on Windows.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	runner := newFakeRunner()
	runner.lookPaths["claude"] = "/usr/local/bin/claude"
	delete(runner.lookPaths, "copilot")
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		stderr:     stderr,
		configSaveFn: func(string, config.Config) error {
			return fmt.Errorf("disk full")
		},
	}
	result, err := resolveEnabledAdapters(deps)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, stderr.String(), "could not save config")
}

func TestResolveAdapters_TargetNotDetectedError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(true),
		},
	})
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	delete(runner.lookPaths, "claude")
	stderr := &bytes.Buffer{}

	deps := &adapterResolverDeps{
		runner:     runner,
		configPath: cfgPath,
		target:     "claude",
		stderr:     stderr,
	}
	_, err := resolveEnabledAdapters(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}



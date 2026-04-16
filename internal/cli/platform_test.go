package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformList_BothDetected_NoConfig(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"

	dir := t.TempDir()
	stdout := &bytes.Buffer{}
	deps := &platformDeps{
		runner:     runner,
		stdout:     stdout,
		stderr:     &bytes.Buffer{},
		configPath: filepath.Join(dir, "config.yaml"),
		noColor:    true,
	}

	err := runPlatformList(deps)
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "claude")
	assert.Contains(t, out, "copilot")
	assert.Contains(t, out, "detected")
}

func TestPlatformList_OneDisabledOneEnabled(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, config.Save(cfgPath, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(true),
			Claude:  boolPtr(false),
		},
	}))

	stdout := &bytes.Buffer{}
	deps := &platformDeps{
		runner:     runner,
		stdout:     stdout,
		stderr:     &bytes.Buffer{},
		configPath: cfgPath,
		noColor:    true,
	}

	err := runPlatformList(deps)
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "enabled")
	assert.Contains(t, out, "disabled")
}

func TestPlatformList_EnabledButNotInstalled(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	delete(runner.lookPaths, "claude")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, config.Save(cfgPath, config.Config{
		Platforms: config.Platforms{
			Copilot: boolPtr(true),
			Claude:  boolPtr(true),
		},
	}))

	stdout := &bytes.Buffer{}
	deps := &platformDeps{
		runner:     runner,
		stdout:     stdout,
		stderr:     &bytes.Buffer{},
		configPath: cfgPath,
		noColor:    true,
	}

	err := runPlatformList(deps)
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "not installed")
}

func TestPlatformEnable(t *testing.T) {
	runner := newFakeRunner()
	runner.lookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.lookPaths["claude"] = "/usr/local/bin/claude"

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	stdout := &bytes.Buffer{}
	deps := &platformDeps{
		runner:     runner,
		stdout:     stdout,
		stderr:     &bytes.Buffer{},
		configPath: cfgPath,
		noColor:    true,
	}

	err := runPlatformToggle("copilot", true, deps)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "copilot enabled")

	// Verify config was saved
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(true), cfg.Platforms.Copilot)
}

func TestPlatformDisable(t *testing.T) {
	runner := newFakeRunner()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, config.Save(cfgPath, config.Config{
		Platforms: config.Platforms{
			Claude: boolPtr(true),
		},
	}))

	stdout := &bytes.Buffer{}
	deps := &platformDeps{
		runner:     runner,
		stdout:     stdout,
		stderr:     &bytes.Buffer{},
		configPath: cfgPath,
		noColor:    true,
	}

	err := runPlatformToggle("claude", false, deps)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "claude disabled")

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(false), cfg.Platforms.Claude)
}

func TestPlatformToggle_UnknownPlatform(t *testing.T) {
	runner := newFakeRunner()
	deps := &platformDeps{
		runner:     runner,
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
		noColor:    true,
	}

	err := runPlatformToggle("vscode", true, deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown platform")
}

func TestPlatformEnable_WarnsIfNotInstalled(t *testing.T) {
	runner := newFakeRunner()
	delete(runner.lookPaths, "claude")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	stderr := &bytes.Buffer{}
	deps := &platformDeps{
		runner:     runner,
		stdout:     &bytes.Buffer{},
		stderr:     stderr,
		configPath: cfgPath,
		noColor:    true,
	}

	err := runPlatformToggle("claude", true, deps)
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "not installed")
}

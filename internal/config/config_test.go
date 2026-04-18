package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		platform   string
		wantEnable bool
		wantConfig bool
	}{
		{
			name:       "copilot enabled",
			cfg:        Config{Platforms: Platforms{Copilot: boolPtr(true)}},
			platform:   "copilot",
			wantEnable: true,
			wantConfig: true,
		},
		{
			name:       "copilot explicitly disabled",
			cfg:        Config{Platforms: Platforms{Copilot: boolPtr(false)}},
			platform:   "copilot",
			wantEnable: false,
			wantConfig: true,
		},
		{
			name:       "copilot not configured",
			cfg:        Config{},
			platform:   "copilot",
			wantEnable: false,
			wantConfig: false,
		},
		{
			name:       "claude enabled",
			cfg:        Config{Platforms: Platforms{Claude: boolPtr(true)}},
			platform:   "claude",
			wantEnable: true,
			wantConfig: true,
		},
		{
			name:       "unknown platform",
			cfg:        Config{Platforms: Platforms{Copilot: boolPtr(true)}},
			platform:   "unknown",
			wantEnable: false,
			wantConfig: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, configured := tt.cfg.IsEnabled(tt.platform)
			assert.Equal(t, tt.wantEnable, enabled)
			assert.Equal(t, tt.wantConfig, configured)
		})
	}
}

func TestConfig_SetPlatform(t *testing.T) {
	var cfg Config

	require.NoError(t, cfg.SetPlatform("copilot", true))
	assert.Equal(t, boolPtr(true), cfg.Platforms.Copilot)

	require.NoError(t, cfg.SetPlatform("claude", false))
	assert.Equal(t, boolPtr(false), cfg.Platforms.Claude)

	err := cfg.SetPlatform("unknown", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown platform")
}

func TestConfig_HasPlatforms(t *testing.T) {
	assert.False(t, (&Config{}).HasPlatforms())
	assert.True(t, (&Config{Platforms: Platforms{Copilot: boolPtr(true)}}).HasPlatforms())
	assert.True(t, (&Config{Platforms: Platforms{Claude: boolPtr(false)}}).HasPlatforms())
}

func TestConfig_EnabledPlatforms(t *testing.T) {
	cfg := Config{Platforms: Platforms{
		Copilot: boolPtr(true),
		Claude:  boolPtr(false),
	}}
	assert.Equal(t, []string{"copilot"}, cfg.EnabledPlatforms())

	cfg2 := Config{Platforms: Platforms{
		Copilot: boolPtr(true),
		Claude:  boolPtr(true),
	}}
	assert.Equal(t, []string{"claude", "copilot"}, cfg2.EnabledPlatforms())

	cfg3 := Config{}
	assert.Nil(t, cfg3.EnabledPlatforms())
}

func TestKnownPlatforms(t *testing.T) {
	known := KnownPlatforms()
	assert.Contains(t, known, "copilot")
	assert.Contains(t, known, "claude")
	assert.Len(t, known, 2)
}

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	require.NoError(t, err)
	assert.False(t, cfg.HasPlatforms())
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("platforms:\n  copilot: true\n  claude: false\n"), 0o644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(true), cfg.Platforms.Copilot)
	assert.Equal(t, boolPtr(false), cfg.Platforms.Claude)
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.False(t, cfg.HasPlatforms())
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(":::invalid"), 0o644))

	_, err := Load(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestLoad_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("platforms:\n  copilot: true\n"), 0o644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(true), cfg.Platforms.Copilot)
	assert.Nil(t, cfg.Platforms.Claude)
}

func TestSave_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.yaml")

	cfg := Config{Platforms: Platforms{Copilot: boolPtr(true), Claude: boolPtr(false)}}
	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(true), loaded.Platforms.Copilot)
	assert.Equal(t, boolPtr(false), loaded.Platforms.Claude)
}

func TestSave_AtomicOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write initial config
	cfg1 := Config{Platforms: Platforms{Copilot: boolPtr(true)}}
	require.NoError(t, Save(path, cfg1))

	// Overwrite
	cfg2 := Config{Platforms: Platforms{Copilot: boolPtr(false), Claude: boolPtr(true)}}
	require.NoError(t, Save(path, cfg2))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(false), loaded.Platforms.Copilot)
	assert.Equal(t, boolPtr(true), loaded.Platforms.Claude)
}

func TestSave_NoTempFileLeftOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Config{Platforms: Platforms{Copilot: boolPtr(true)}}
	require.NoError(t, Save(path, cfg))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only config.yaml should remain, no temp files")
	assert.Equal(t, "config.yaml", entries[0].Name())
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(p))
	assert.True(t, strings.HasSuffix(p, filepath.Join(".summon", "config.yaml")))
}

func TestLoad_ReadError(t *testing.T) {
	// Passing a directory to Load triggers a non-ErrNotExist read error
	dir := t.TempDir()
	_, err := Load(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading config")
}

func TestSave_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file where a directory would be needed
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	path := filepath.Join(blocker, "sub", "config.yaml")
	err := Save(path, Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating config directory")
}

func TestSave_CreateTempError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based test not applicable on Windows")
	}
	dir := t.TempDir()
	configDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.Chmod(configDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o755) })

	path := filepath.Join(configDir, "config.yaml")
	err := Save(path, Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating temp file")
}

func TestSave_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Create a non-empty directory at the target path so rename(file→dir) fails
	targetPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.MkdirAll(targetPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetPath, "dummy"), []byte("x"), 0o644))

	err := Save(targetPath, Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "renaming config")
}

func TestRoundTrip_OmitsNilFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Config{Platforms: Platforms{Copilot: boolPtr(true)}}
	require.NoError(t, Save(path, cfg))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "copilot: true")
	assert.NotContains(t, content, "claude")
}

package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/summon/internal/registry"
)

// helper to write a minimal summon.yaml into a package directory.
func writeSummonYAML(t *testing.T, dir, name, version, description string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "name: " + name + "\nversion: " + version + "\ndescription: " + description + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(content), 0o644))
}

func writeSummonYAMLWithPlatforms(t *testing.T, dir, name, version, description string, platforms []string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "name: " + name + "\nversion: " + version + "\ndescription: " + description + "\nplatforms:\n"
	for _, p := range platforms {
		content += "  - " + p + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(content), 0o644))
}

func TestGenerate_CreatesMarketplaceJSON(t *testing.T) {
	storeDir := t.TempDir()
	platformDir := filepath.Join(t.TempDir(), "claude")

	reg := registry.New()
	reg.Add("my-plugin", registry.Entry{Version: "1.0.0"})
	writeSummonYAML(t, filepath.Join(storeDir, "my-plugin"), "my-plugin", "1.0.0", "A test plugin")

	err := Generate("claude", "test-marketplace", storeDir, platformDir, reg)
	require.NoError(t, err)

	// marketplace.json must live inside .claude-plugin/
	data, err := os.ReadFile(filepath.Join(platformDir, ".claude-plugin", "marketplace.json"))
	require.NoError(t, err)

	var m Marketplace
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "test-marketplace", m.Name)
	assert.NotEmpty(t, m.Description)
	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "my-plugin", m.Plugins[0].Name)
	assert.Equal(t, "1.0.0", m.Plugins[0].Version)
	assert.Equal(t, "A test plugin", m.Plugins[0].Description)
	assert.Equal(t, "./plugins/my-plugin", m.Plugins[0].Source)

	// Symlink should exist
	link := filepath.Join(platformDir, "plugins", "my-plugin")
	fi, err := os.Lstat(link)
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink != 0, "expected symlink")
}

func TestGenerate_EmptyRegistry(t *testing.T) {
	storeDir := t.TempDir()
	platformDir := filepath.Join(t.TempDir(), "claude")

	reg := registry.New()

	err := Generate("claude", "empty-market", storeDir, platformDir, reg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(platformDir, ".claude-plugin", "marketplace.json"))
	require.NoError(t, err)

	var m Marketplace
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "empty-market", m.Name)
	assert.Empty(t, m.Plugins)
}

func TestGenerate_PlatformFiltering(t *testing.T) {
	storeDir := t.TempDir()
	claudeDir := filepath.Join(t.TempDir(), "claude")
	copilotDir := filepath.Join(t.TempDir(), "copilot")

	reg := registry.New()
	reg.Add("claude-only", registry.Entry{Version: "1.0.0", Platforms: []string{"claude"}})
	writeSummonYAMLWithPlatforms(t, filepath.Join(storeDir, "claude-only"), "claude-only", "1.0.0", "Claude only plugin", []string{"claude"})

	require.NoError(t, Generate("claude", "claude-mkt", storeDir, claudeDir, reg))
	require.NoError(t, Generate("copilot", "copilot-mkt", storeDir, copilotDir, reg))

	// Should appear in claude marketplace
	data, _ := os.ReadFile(filepath.Join(claudeDir, ".claude-plugin", "marketplace.json"))
	var claudeMkt Marketplace
	require.NoError(t, json.Unmarshal(data, &claudeMkt))
	assert.Len(t, claudeMkt.Plugins, 1)

	// Should NOT appear in copilot marketplace
	data, _ = os.ReadFile(filepath.Join(copilotDir, ".claude-plugin", "marketplace.json"))
	var copilotMkt Marketplace
	require.NoError(t, json.Unmarshal(data, &copilotMkt))
	assert.Empty(t, copilotMkt.Plugins)
}

func TestGenerate_NoPlatformsMatchesAll(t *testing.T) {
	storeDir := t.TempDir()
	claudeDir := filepath.Join(t.TempDir(), "claude")
	copilotDir := filepath.Join(t.TempDir(), "copilot")

	reg := registry.New()
	reg.Add("universal", registry.Entry{Version: "2.0.0", Platforms: nil})
	writeSummonYAML(t, filepath.Join(storeDir, "universal"), "universal", "2.0.0", "Works everywhere")

	require.NoError(t, Generate("claude", "mkt", storeDir, claudeDir, reg))
	require.NoError(t, Generate("copilot", "mkt", storeDir, copilotDir, reg))

	data, _ := os.ReadFile(filepath.Join(claudeDir, ".claude-plugin", "marketplace.json"))
	var cm Marketplace
	require.NoError(t, json.Unmarshal(data, &cm))
	assert.Len(t, cm.Plugins, 1)

	data, _ = os.ReadFile(filepath.Join(copilotDir, ".claude-plugin", "marketplace.json"))
	var vm Marketplace
	require.NoError(t, json.Unmarshal(data, &vm))
	assert.Len(t, vm.Plugins, 1)
}

func TestGenerate_FallbackManifest(t *testing.T) {
	storeDir := t.TempDir()
	platformDir := filepath.Join(t.TempDir(), "claude")

	// Create package dir without summon.yaml so it falls back to registry data
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "no-manifest"), 0o755))

	reg := registry.New()
	reg.Add("no-manifest", registry.Entry{Version: "0.5.0"})

	err := Generate("claude", "fallback-mkt", storeDir, platformDir, reg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(platformDir, ".claude-plugin", "marketplace.json"))
	require.NoError(t, err)

	var m Marketplace
	require.NoError(t, json.Unmarshal(data, &m))

	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "no-manifest", m.Plugins[0].Name)
	assert.Equal(t, "0.5.0", m.Plugins[0].Version)
	assert.Equal(t, "./plugins/no-manifest", m.Plugins[0].Source)
}

func TestPlatformMatch(t *testing.T) {
	tests := []struct {
		name      string
		platforms []string
		platform  string
		want      bool
	}{
		{"empty list matches all", nil, "claude", true},
		{"empty slice matches all", []string{}, "copilot", true},
		{"matching platform", []string{"claude", "copilot"}, "claude", true},
		{"non-matching platform", []string{"claude"}, "copilot", false},
		{"single match", []string{"copilot"}, "copilot", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, platformMatch(tt.platforms, tt.platform))
		})
	}
}

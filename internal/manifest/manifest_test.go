package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_ValidManifest(t *testing.T) {
	yaml := []byte("name: my-package\nversion: \"1.0.0\"\ndescription: \"A test package\"\n")
	m, err := Parse(yaml)
	require.NoError(t, err)
	assert.Equal(t, "my-package", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, "A test package", m.Description)
}

func TestParse_MissingName(t *testing.T) {
	yaml := []byte("version: \"1.0.0\"\ndescription: \"A test package\"\n")
	_, err := Parse(yaml)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestParse_MissingVersion(t *testing.T) {
	yaml := []byte("name: my-package\ndescription: \"A test package\"\n")
	_, err := Parse(yaml)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestParse_MissingDescription(t *testing.T) {
	yaml := []byte("name: my-package\nversion: \"1.0.0\"\n")
	_, err := Parse(yaml)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "description is required")
}

func TestValidateFull_KebabCase(t *testing.T) {
	m := &Manifest{Name: "My_Package", Version: "1.0.0", Description: "test"}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "name must be kebab-case (e.g., my-package)")
}

func TestValidateFull_InvalidSemver(t *testing.T) {
	m := &Manifest{Name: "my-pkg", Version: "abc", Description: "test"}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "version must be valid semver (MAJOR.MINOR.PATCH)")
}

func TestValidateFull_UnknownPlatform(t *testing.T) {
	m := &Manifest{Name: "my-pkg", Version: "1.0.0", Description: "test", Platforms: []string{"unknown"}}
	errs := m.ValidateFull("")
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "unknown platform")
}

func TestValidateFull_Valid(t *testing.T) {
	m := &Manifest{Name: "my-pkg", Version: "1.0.0", Description: "test", Platforms: []string{"claude"}}
	errs := m.ValidateFull("")
	assert.Empty(t, errs)
}

func TestValidateFull_ComponentPathMissing(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Name: "my-pkg", Version: "1.0.0", Description: "test",
		Components: &Components{Skills: "nonexistent/"},
	}
	errs := m.ValidateFull(dir)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "component skills path")
}

func TestCheckSummonVersion_NoConstraint(t *testing.T) {
	ok, _ := CheckSummonVersion("", "0.1.0")
	assert.True(t, ok)
}

func TestCheckSummonVersion_Satisfied(t *testing.T) {
	ok, _ := CheckSummonVersion(">=0.1.0", "0.2.0")
	assert.True(t, ok)
}

func TestCheckSummonVersion_NotSatisfied(t *testing.T) {
	ok, msg := CheckSummonVersion(">=1.0.0", "0.1.0")
	assert.False(t, ok)
	assert.Contains(t, msg, "requires summon >=1.0.0")
}

func TestLoad_FromDirectory(t *testing.T) {
	dir := t.TempDir()
	content := []byte("name: test-pkg\nversion: \"1.0.0\"\ndescription: test\n")
	err := os.WriteFile(filepath.Join(dir, "summon.yaml"), content, 0o644)
	require.NoError(t, err)
	m, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "test-pkg", m.Name)
}

func TestLoad_NoManifest(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no summon.yaml found")
}

func TestParse_InvalidYAML(t *testing.T) {
	data := []byte(":\n\t- bad:\nyaml: [unterminated")
	_, err := Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing summon.yaml")
}

func TestParse_AllOptionalFields(t *testing.T) {
	data := []byte(`
name: full-pkg
version: "2.1.0"
description: "A fully populated package"
author:
  name: Jane Doe
  email: jane@example.com
license: MIT
homepage: https://example.com
repository: https://github.com/user/full-pkg
keywords:
  - ai
  - tools
platforms:
  - claude
  - copilot
components:
  skills: skills/
  agents: agents/
dependencies:
  other-pkg: ">=1.0.0"
`)
	m, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "full-pkg", m.Name)
	assert.Equal(t, "2.1.0", m.Version)
	assert.Equal(t, "A fully populated package", m.Description)
	require.NotNil(t, m.Author)
	assert.Equal(t, "Jane Doe", m.Author.Name)
	assert.Equal(t, "jane@example.com", m.Author.Email)
	assert.Equal(t, "MIT", m.License)
	assert.Equal(t, "https://example.com", m.Homepage)
	assert.Equal(t, "https://github.com/user/full-pkg", m.Repository)
	assert.Equal(t, []string{"ai", "tools"}, m.Keywords)
	assert.Equal(t, []string{"claude", "copilot"}, m.Platforms)
	require.NotNil(t, m.Components)
	assert.Equal(t, "skills/", m.Components.Skills)
	assert.Equal(t, "agents/", m.Components.Agents)
	assert.Equal(t, ">=1.0.0", m.Dependencies["other-pkg"])
}

func TestValidateFull_NameTooLong(t *testing.T) {
	longName := "a"
	for len(longName) <= 64 {
		longName += "a"
	}
	m := &Manifest{Name: longName, Version: "1.0.0", Description: "test"}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "name must be 64 characters or fewer")
}

func TestValidateFull_DescriptionTooLong(t *testing.T) {
	longDesc := strings.Repeat("x", 257)
	m := &Manifest{Name: "my-pkg", Version: "1.0.0", Description: longDesc}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "description must be 256 characters or fewer")
}

func TestValidateFull_ValidComponentPaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
	m := &Manifest{
		Name: "my-pkg", Version: "1.0.0", Description: "test",
		Components: &Components{Skills: "skills", Agents: "agents"},
	}
	errs := m.ValidateFull(dir)
	assert.Empty(t, errs)
}

func TestValidateFull_EmptyName(t *testing.T) {
	m := &Manifest{Name: "", Version: "1.0.0", Description: "test"}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "name is required")
}

func TestValidateFull_EmptyVersion(t *testing.T) {
	m := &Manifest{Name: "my-pkg", Version: "", Description: "test"}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "version is required")
}

func TestValidateFull_EmptyDescription(t *testing.T) {
	m := &Manifest{Name: "my-pkg", Version: "1.0.0", Description: ""}
	errs := m.ValidateFull("")
	assert.Contains(t, errs, "description is required")
}

func TestIsValidSemver(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1.0.0", true},
		{"v1.0.0", true},
		{"1.0.0-beta", true},
		{"1.0", false},
		{"abc", false},
		{"", false},
		{"1.0.0.0", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, isValidSemver(tc.input))
		})
	}
}

func TestCheckSummonVersion_EqualVersion(t *testing.T) {
	ok, msg := CheckSummonVersion(">=1.0.0", "1.0.0")
	assert.True(t, ok)
	assert.Empty(t, msg)
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "1.2.3", "1.2.3", 0},
		{"major less", "0.9.9", "1.0.0", -1},
		{"major greater", "2.0.0", "1.9.9", 1},
		{"minor less", "1.0.0", "1.1.0", -1},
		{"minor greater", "1.2.0", "1.1.0", 1},
		{"patch less", "1.0.0", "1.0.1", -1},
		{"patch greater", "1.0.2", "1.0.1", 1},
		{"with v prefix", "v1.0.0", "1.0.0", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, compareSemver(tc.a, tc.b))
		})
	}
}

// ---------------------------------------------------------------------------
// plugin.json fallback tests (US1)
// ---------------------------------------------------------------------------

// helper: create a plugin.json in dir/.claude-plugin/plugin.json
func writePluginJSON(t *testing.T, dir string, data map[string]any) {
	t.Helper()
	pluginDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	b, err := json.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), b, 0o644))
}

func TestInferFromPluginJSON_AutoDetectComponents(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, map[string]any{
		"name": "detect-test", "version": "1.0.0", "description": "test",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "commands"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hooks.json"), []byte("{}"), 0o644))

	m, err := inferFromPluginJSON(dir)
	require.NoError(t, err)
	require.NotNil(t, m.Components)
	assert.Equal(t, "skills", m.Components.Skills)
	assert.Equal(t, "commands", m.Components.Commands)
	assert.Equal(t, ".", m.Components.Hooks)
	assert.Empty(t, m.Components.Agents)
	assert.Empty(t, m.Components.MCP)
}

func TestInferFromPluginJSON_NoComponents(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, map[string]any{
		"name": "bare-test", "version": "0.1.0", "description": "no components",
	})

	m, err := inferFromPluginJSON(dir)
	require.NoError(t, err)
	assert.Nil(t, m.Components)
	assert.Equal(t, "bare-test", m.Name)
}

func TestInferFromPluginJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("{bad json"), 0o644))

	_, err := inferFromPluginJSON(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing plugin.json")
}

func TestInferFromPluginJSON_HooksSubdir(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, map[string]any{
		"name": "hooks-test", "version": "1.0.0", "description": "test",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hooks", "hooks.json"), []byte("{}"), 0o644))

	m, err := inferFromPluginJSON(dir)
	require.NoError(t, err)
	require.NotNil(t, m.Components)
	assert.Equal(t, "hooks", m.Components.Hooks)
}

func TestLoadOrInfer_PluginJSON(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, map[string]any{
		"name": "pj-test", "version": "2.0.0", "description": "plugin.json only",
		"author":  map[string]any{"name": "Alice"},
		"license": "MIT", "homepage": "https://example.com",
	})

	manifests, roots, err := LoadOrInfer(dir)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	require.Len(t, roots, 1)

	m := manifests[0]
	assert.Equal(t, "pj-test", m.Name)
	assert.Equal(t, "2.0.0", m.Version)
	assert.Equal(t, "plugin.json only", m.Description)
	assert.Equal(t, []string{"claude", "copilot"}, m.Platforms)
	assert.Equal(t, "MIT", m.License)
	assert.Equal(t, "https://example.com", m.Homepage)
	require.NotNil(t, m.Author)
	assert.Equal(t, "Alice", m.Author.Name)
	assert.Equal(t, dir, roots[0])
}

func TestLoadOrInfer_NothingFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := LoadOrInfer(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no summon.yaml, plugin.json, or marketplace.json")
}

// ---------------------------------------------------------------------------
// marketplace.json fallback tests (US2)
// ---------------------------------------------------------------------------

func writeMarketplaceJSON(t *testing.T, dir string, mj map[string]any) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	b, err := json.Marshal(mj)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "marketplace.json"), b, 0o644))
}

func TestLoadOrInfer_MarketplaceJSON(t *testing.T) {
	dir := t.TempDir()
	sub1 := filepath.Join(dir, "plugin-a")
	sub2 := filepath.Join(dir, "plugin-b")
	writePluginJSON(t, sub1, map[string]any{
		"name": "plugin-a", "version": "1.0.0", "description": "first",
	})
	writePluginJSON(t, sub2, map[string]any{
		"name": "plugin-b", "version": "2.0.0", "description": "second",
	})
	writeMarketplaceJSON(t, dir, map[string]any{
		"name": "my-marketplace",
		"plugins": []map[string]any{
			{"name": "plugin-a", "source": "./plugin-a"},
			{"name": "plugin-b", "source": "./plugin-b"},
		},
	})

	manifests, roots, err := LoadOrInfer(dir)
	require.NoError(t, err)
	require.Len(t, manifests, 2)
	require.Len(t, roots, 2)
	assert.Equal(t, "plugin-a", manifests[0].Name)
	assert.Equal(t, "plugin-b", manifests[1].Name)
	assert.Equal(t, sub1, roots[0])
	assert.Equal(t, sub2, roots[1])
}

func TestLoadOrInfer_PluginJSONBeatsMarketplace(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, map[string]any{
		"name": "direct-plugin", "version": "1.0.0", "description": "direct",
	})
	writeMarketplaceJSON(t, dir, map[string]any{
		"name": "mp",
		"plugins": []map[string]any{
			{"name": "mp-plugin", "source": "./sub"},
		},
	})

	manifests, roots, err := LoadOrInfer(dir)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	assert.Equal(t, "direct-plugin", manifests[0].Name)
	assert.Equal(t, dir, roots[0])
}

func TestInferFromMarketplaceJSON_EmptyPlugins(t *testing.T) {
	dir := t.TempDir()
	writeMarketplaceJSON(t, dir, map[string]any{
		"name": "empty", "plugins": []map[string]any{},
	})
	_, _, err := inferFromMarketplaceJSON(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no plugins listed")
}

func TestInferFromMarketplaceJSON_MissingPluginJSON(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "missing-pj")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	writeMarketplaceJSON(t, dir, map[string]any{
		"name": "mp",
		"plugins": []map[string]any{
			{"name": "x", "source": "./missing-pj"},
		},
	})
	_, _, err := inferFromMarketplaceJSON(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-pj")
}

func TestInferFromMarketplaceJSON_DuplicateNames(t *testing.T) {
	dir := t.TempDir()
	sub1 := filepath.Join(dir, "a")
	sub2 := filepath.Join(dir, "b")
	writePluginJSON(t, sub1, map[string]any{
		"name": "dup-name", "version": "1.0.0", "description": "first",
	})
	writePluginJSON(t, sub2, map[string]any{
		"name": "dup-name", "version": "2.0.0", "description": "second",
	})
	writeMarketplaceJSON(t, dir, map[string]any{
		"name": "mp",
		"plugins": []map[string]any{
			{"name": "a", "source": "./a"},
			{"name": "b", "source": "./b"},
		},
	})
	_, _, err := inferFromMarketplaceJSON(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
}

func TestInferFromMarketplaceJSON_SummonYamlPreferred(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "my-plugin")

	// plugin.json exists but summon.yaml should win
	writePluginJSON(t, sub, map[string]any{
		"name": "my-plugin", "version": "1.0.0", "description": "from plugin.json",
	})
	require.NoError(t, os.WriteFile(filepath.Join(sub, "summon.yaml"),
		[]byte("name: my-plugin\nversion: \"2.0.0\"\ndescription: from summon.yaml\nplatforms:\n  - claude\ndependencies:\n  superpowers: \">=5.0.7\"\n"),
		0o644,
	))
	writeMarketplaceJSON(t, dir, map[string]any{
		"name": "mp",
		"plugins": []map[string]any{
			{"name": "my-plugin", "source": "./my-plugin"},
		},
	})

	manifests, roots, err := inferFromMarketplaceJSON(dir)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	assert.Equal(t, "my-plugin", manifests[0].Name)
	assert.Equal(t, "2.0.0", manifests[0].Version)
	assert.Equal(t, "from summon.yaml", manifests[0].Description)
	assert.Equal(t, map[string]string{"superpowers": ">=5.0.7"}, manifests[0].Dependencies)
	assert.Equal(t, sub, roots[0])
}

// ---------------------------------------------------------------------------
// US3: summon.yaml priority tests
// ---------------------------------------------------------------------------

func TestLoadOrInfer_SummonYamlWins(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"),
		[]byte("name: yaml-pkg\nversion: \"3.0.0\"\ndescription: from yaml\nplatforms:\n  - claude\n"),
		0o644,
	))
	writePluginJSON(t, dir, map[string]any{
		"name": "json-pkg", "version": "1.0.0", "description": "from json",
	})

	manifests, roots, err := LoadOrInfer(dir)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	m := manifests[0]
	assert.Equal(t, "yaml-pkg", m.Name)
	assert.Equal(t, "3.0.0", m.Version)
	assert.Equal(t, []string{"claude"}, m.Platforms)
	assert.Equal(t, dir, roots[0])
}

func TestLoadOrInfer_SummonYaml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"),
		[]byte("name: yaml-only\nversion: \"1.0.0\"\ndescription: test\n"),
		0o644,
	))

	manifests, roots, err := LoadOrInfer(dir)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	assert.Equal(t, "yaml-only", manifests[0].Name)
	assert.Equal(t, dir, roots[0])
}

// ---------------------------------------------------------------------------
// ValidateInferred
// ---------------------------------------------------------------------------

func TestValidateInferred(t *testing.T) {
	m := &Manifest{Name: "x", Version: "1.0.0", Description: "d"}
	assert.NoError(t, m.ValidateInferred())

	m2 := &Manifest{Name: "", Version: "1.0.0", Description: "d"}
	err := m2.ValidateInferred()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

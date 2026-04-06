package manifest

import (
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

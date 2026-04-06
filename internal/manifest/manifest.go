// Package manifest handles parsing and validation of summon.yaml, the package
// specification file that describes a summon package's metadata, components,
// platform targets, and dependency requirements.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Author represents a package author.
type Author struct {
	Name  string `yaml:"name" json:"name"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
}

// Components declares which AI agent component types the package provides.
// Each field is a relative directory path within the package that contains
// the corresponding component type (skills, agents, commands, hooks, or MCP servers).
type Components struct {
	Skills   string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents   string `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands string `yaml:"commands,omitempty" json:"commands,omitempty"`
	Hooks    string `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	MCP      string `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

// Manifest represents a summon.yaml package manifest.
type Manifest struct {
	Name          string            `yaml:"name"`
	Version       string            `yaml:"version"`
	Description   string            `yaml:"description"`
	Author        *Author           `yaml:"author,omitempty"`
	License       string            `yaml:"license,omitempty"`
	Homepage      string            `yaml:"homepage,omitempty"`
	Repository    string            `yaml:"repository,omitempty"`
	Keywords      []string          `yaml:"keywords,omitempty"`
	Platforms     []string          `yaml:"platforms,omitempty"`
	Components    *Components       `yaml:"components,omitempty"`
	SummonVersion string            `yaml:"summon_version,omitempty"`
	Dependencies  map[string]string `yaml:"dependencies,omitempty"`
}

// Load reads and parses a summon.yaml from the given directory.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "summon.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no summon.yaml found in %s", dir)
		}
		return nil, fmt.Errorf("reading summon.yaml: %w", err)
	}
	return Parse(data)
}

// Parse parses summon.yaml content.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing summon.yaml: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks required fields.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("summon.yaml: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("summon.yaml: version is required")
	}
	if m.Description == "" {
		return fmt.Errorf("summon.yaml: description is required")
	}
	return nil
}

var kebabCaseRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateFull performs comprehensive manifest validation including name length
// and format, semver version, description length, platform allowlist, and
// component directory existence. It returns a slice of human-readable error
// strings (empty when the manifest is valid).
func (m *Manifest) ValidateFull(pkgDir string) []string {
	var errs []string

	if m.Name == "" {
		errs = append(errs, "name is required")
	} else {
		if len(m.Name) > 64 {
			errs = append(errs, "name must be 64 characters or fewer")
		}
		if !kebabCaseRegex.MatchString(m.Name) {
			errs = append(errs, "name must be kebab-case (e.g., my-package)")
		}
	}

	if m.Version == "" {
		errs = append(errs, "version is required")
	} else if !isValidSemver(m.Version) {
		errs = append(errs, "version must be valid semver (MAJOR.MINOR.PATCH)")
	}

	if m.Description == "" {
		errs = append(errs, "description is required")
	} else if len(m.Description) > 256 {
		errs = append(errs, "description must be 256 characters or fewer")
	}

	validPlatforms := map[string]bool{"claude": true, "copilot": true}
	for _, p := range m.Platforms {
		if !validPlatforms[p] {
			errs = append(errs, fmt.Sprintf("unknown platform: %q", p))
		}
	}

	if m.Components != nil && pkgDir != "" {
		checkComponentPath := func(label, path string) {
			if path == "" {
				return
			}
			full := filepath.Join(pkgDir, path)
			if _, err := os.Stat(full); err != nil {
				errs = append(errs, fmt.Sprintf("component %s path %q does not exist", label, path))
			}
		}
		checkComponentPath("skills", m.Components.Skills)
		checkComponentPath("agents", m.Components.Agents)
		checkComponentPath("commands", m.Components.Commands)
		checkComponentPath("hooks", m.Components.Hooks)
		checkComponentPath("mcp", m.Components.MCP)
	}

	return errs
}

// isValidSemver checks whether v is a valid semantic version string
// (MAJOR.MINOR.PATCH) with an optional "v" prefix and pre-release suffix.
func isValidSemver(v string) bool {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// GetName returns the package name, satisfying platform.ComponentsInfo.
func (m *Manifest) GetName() string { return m.Name }

// GetSkills returns the skills component directory, satisfying platform.ComponentsInfo.
func (m *Manifest) GetSkills() string {
	if m.Components == nil {
		return ""
	}
	return m.Components.Skills
}

// GetAgents returns the agents component directory, satisfying platform.ComponentsInfo.
func (m *Manifest) GetAgents() string {
	if m.Components == nil {
		return ""
	}
	return m.Components.Agents
}

// GetHooks returns the hooks component directory, satisfying platform.ComponentsInfo.
func (m *Manifest) GetHooks() string {
	if m.Components == nil {
		return ""
	}
	return m.Components.Hooks
}

// GetMCP returns the MCP component directory, satisfying platform.ComponentsInfo.
func (m *Manifest) GetMCP() string {
	if m.Components == nil {
		return ""
	}
	return m.Components.MCP
}

// CheckSummonVersion checks if the running summon version satisfies the manifest constraint.
func CheckSummonVersion(constraint, currentVersion string) (bool, string) {
	if constraint == "" {
		return true, ""
	}
	constraint = strings.TrimSpace(constraint)
	if strings.HasPrefix(constraint, ">=") {
		required := strings.TrimPrefix(constraint, ">=")
		if compareSemver(currentVersion, required) >= 0 {
			return true, ""
		}
		return false, fmt.Sprintf("requires summon %s, running %s", constraint, currentVersion)
	}
	return true, ""
}

// compareSemver compares two semver strings and returns -1, 0, or 1 analogous
// to strings.Compare.
func compareSemver(a, b string) int {
	aParts := parseSemverParts(a)
	bParts := parseSemverParts(b)
	for i := 0; i < 3; i++ {
		if aParts[i] != bParts[i] {
			if aParts[i] < bParts[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseSemverParts extracts the [major, minor, patch] integers from a semver
// string, stripping any leading "v" prefix and trailing pre-release suffix.
func parseSemverParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	var result [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		result[i], _ = strconv.Atoi(parts[i])
	}
	return result
}

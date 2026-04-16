package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SystemRequirement represents a system binary dependency.
type SystemRequirement struct {
	Name     string `yaml:"name"`
	Optional bool   `yaml:"optional"`
	Reason   string `yaml:"reason"`
}

// UnmarshalYAML supports both string and object forms for system requirements.
func (s *SystemRequirement) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		s.Name = value.Value
		s.Optional = false
		return nil
	}

	type rawSysReq struct {
		Name     string `yaml:"name"`
		Optional bool   `yaml:"optional"`
		Reason   string `yaml:"reason"`
	}
	var raw rawSysReq
	if err := value.Decode(&raw); err != nil {
		return err
	}
	s.Name = raw.Name
	s.Optional = raw.Optional
	s.Reason = raw.Reason
	return nil
}

// Manifest represents a summon.yaml file.
// It declares dependencies, system requirements, and marketplace overrides.
// name and description are not included — they are metadata concerns
// handled by the marketplace index, not the dependency manifest.
type Manifest struct {
	Marketplaces       map[string]string   `yaml:"marketplaces,omitempty"`
	Dependencies       []string            `yaml:"dependencies,omitempty"`
	SystemRequirements []SystemRequirement `yaml:"system_requirements,omitempty"`
}

// Validate checks that the manifest has valid values.
func (m *Manifest) Validate() error {
	for i, sr := range m.SystemRequirements {
		if sr.Name == "" {
			return fmt.Errorf("manifest validation: system_requirements[%d] must have a 'name'", i)
		}
		if sr.Optional && sr.Reason == "" {
			return fmt.Errorf("manifest validation: system_requirements[%d] ('%s') is optional but missing 'reason'", i, sr.Name)
		}
	}

	return nil
}

// Parse reads and parses a summon.yaml from the given bytes.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest parse error: %w", err)
	}
	return &m, nil
}

// ParseAndValidate parses and validates manifest data.
func ParseAndValidate(data []byte) (*Manifest, error) {
	m, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// LoadFile reads and parses a summon.yaml from a file path.
func LoadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest load error: %w", err)
	}
	return ParseAndValidate(data)
}

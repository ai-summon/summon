// Package registry manages registry.yaml, the persistent manifest that tracks
// all installed summon packages, their versions, sources, and installation
// timestamps.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Source describes where a package was installed from.
type Source struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
	Ref  string `yaml:"ref,omitempty"`
	SHA  string `yaml:"sha,omitempty"`
}

// Entry represents one installed package in the registry.
type Entry struct {
	Version     string   `yaml:"version"`
	Source      Source   `yaml:"source"`
	Platforms   []string `yaml:"platforms"`
	InstalledAt string   `yaml:"installed_at"`
}

// Registry represents a registry.yaml file.
type Registry struct {
	SummonVersion string           `yaml:"summon_version"`
	Scope         string           `yaml:"scope,omitempty"`
	Packages      map[string]Entry `yaml:"packages"`
}

// New creates an empty registry.
func New() *Registry {
	return &Registry{
		SummonVersion: "0.1.0",
		Packages:      make(map[string]Entry),
	}
}

// Load reads a registry.yaml from the given path.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("reading registry: %w", err)
	}
	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	if r.Packages == nil {
		r.Packages = make(map[string]Entry)
	}
	return &r, nil
}

// Save writes the registry to disk.
func (r *Registry) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshaling registry: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// Add adds or updates a package entry.
func (r *Registry) Add(name string, entry Entry) {
	if entry.InstalledAt == "" {
		entry.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	}
	r.Packages[name] = entry
}

// Remove removes a package entry.
func (r *Registry) Remove(name string) bool {
	if _, ok := r.Packages[name]; !ok {
		return false
	}
	delete(r.Packages, name)
	return true
}

// Get retrieves a package entry.
func (r *Registry) Get(name string) (Entry, bool) {
	e, ok := r.Packages[name]
	return e, ok
}

// Has checks if a package is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.Packages[name]
	return ok
}

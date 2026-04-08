// Package catalog provides access to the built-in package registry.
//
// It loads a curated list of packages from an embedded YAML file (catalog.yaml)
// and exposes fast name-based lookups. External callers can also supply custom
// YAML data via [Load] for testing or user-defined catalogs.
package catalog

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var defaultCatalog []byte

// Entry describes a single package in the catalog.
type Entry struct {
	Name        string `yaml:"name"`
	Repository  string `yaml:"repository"`
	Description string `yaml:"description,omitempty"`
}

// Catalog holds a list of package entries and an index for O(1) name lookups.
type Catalog struct {
	Entries []Entry `yaml:"packages"`
	byName  map[string]Entry
}

// Load parses YAML data into a Catalog and builds the name index.
// The YAML is expected to have a top-level "packages" key containing a list of entries.
func Load(data []byte) (*Catalog, error) {
	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing catalog: %w", err)
	}
	c.index()
	return &c, nil
}

// LoadDefault loads the embedded default catalog shipped with the binary.
func LoadDefault() (*Catalog, error) {
	return Load(defaultCatalog)
}

// index builds the byName map for fast lookups.
// When duplicate names exist, the last entry wins.
func (c *Catalog) index() {
	c.byName = make(map[string]Entry, len(c.Entries))
	for _, e := range c.Entries {
		c.byName[e.Name] = e
	}
}

// Lookup returns the entry matching name and true, or a zero Entry and false
// if no match is found. The lookup is case-sensitive.
func (c *Catalog) Lookup(name string) (Entry, bool) {
	e, ok := c.byName[name]
	return e, ok
}

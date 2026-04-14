package marketplace

import (
	"encoding/json"
	"fmt"

	"github.com/ai-summon/summon/internal/git"
)

// PackageEntry represents a single entry in a marketplace index.
type PackageEntry struct {
	Source      string `json:"source"`
	Description string `json:"description"`
}

// Index is the marketplace index: package name → entry.
type Index map[string]PackageEntry

// Registry provides marketplace package lookup.
type Registry struct {
	gitClient *git.Client
}

// NewRegistry creates a new Registry.
func NewRegistry(gitClient *git.Client) *Registry {
	return &Registry{gitClient: gitClient}
}

// FetchIndex fetches and parses the marketplace index from a git repo.
// It accepts a fetchFunc to allow injection for testing.
func FetchIndex(data []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace index: %w", err)
	}
	return idx, nil
}

// Lookup finds a package in the index by name.
func (idx Index) Lookup(name string) (*PackageEntry, bool) {
	entry, ok := idx[name]
	if !ok {
		return nil, false
	}
	return &entry, true
}

// OfficialMarketplaceURL is the default marketplace repo.
const OfficialMarketplaceURL = "https://github.com/ai-summon/summon-marketplace"

// MarketplaceSource represents a named marketplace source.
type MarketplaceSource struct {
	Name   string
	Source string
}

// IndexFetcher abstracts marketplace index fetching for dependency injection.
type IndexFetcher interface {
	FetchMarketplaceIndex(source string) (Index, error)
}

// DefaultIndexFetcher fetches marketplace indices using git clone.
type DefaultIndexFetcher struct {
	gitClient *git.Client
	cache     map[string]Index
}

// NewDefaultIndexFetcher creates a new DefaultIndexFetcher.
func NewDefaultIndexFetcher(gitClient *git.Client) *DefaultIndexFetcher {
	return &DefaultIndexFetcher{
		gitClient: gitClient,
		cache:     make(map[string]Index),
	}
}

// FetchMarketplaceIndex fetches a marketplace index from a git source, with caching.
func (f *DefaultIndexFetcher) FetchMarketplaceIndex(source string) (Index, error) {
	if idx, ok := f.cache[source]; ok {
		return idx, nil
	}

	// For now, return an error indicating the marketplace needs to be fetched
	// The actual implementation will clone and parse the repo
	return nil, fmt.Errorf("marketplace fetch not implemented for %s (would clone and parse marketplace.json)", source)
}
